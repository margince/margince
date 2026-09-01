// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// The note a colleague FORWARDS, as distinct from the ask they receive.
//
// Two messages exist in one workflow and they are written for different
// readers. org360's draftIntroRequest writes the internal ask — its own prompt
// says "the message is TO the colleague" — and this writes the prospect-facing
// note that colleague pastes into the mail they send onward. Collapsing them
// would put an internal favour-ask in front of a customer.
//
// It lives in package network because the facts are the graph's: who the
// colleague is, how warm the relationship is, when they last spoke, and whether
// the route runs through somebody. buildPersonGraph is a private method on
// Reads over unexported fields, so a drafter anywhere else would have to
// re-derive all of that — a second answer to "how well do these two know each
// other", free to disagree with the map on screen.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Completer is the model lane, injected. Nil is an ordinary configuration and
// not a failure: the note is then written from the template, which is a note a
// rep can actually use.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// WithIntroNoteLane binds the lane that phrases the forwardable note.
func (h Reads) WithIntroNoteLane(lane Completer) Reads {
	h.introNoteLane = lane
	return h
}

// noteFacts is everything the note may say, and nothing else.
//
// Every field is read from the graph this contact's own page draws, so the note
// cannot claim a closeness the map does not show. What the rep supplies is
// exactly one thing — why it is worth the contact's time — because that is the
// one fact no record holds.
type noteFacts struct {
	// colleague is who the contact will hear from. The note is written in
	// their voice: they are the sender, and the rep is the person being
	// introduced.
	colleague string
	// contact is who the note is addressed to.
	contact string
	// through names the intermediary on a route that runs through another
	// contact, and is empty on a direct one. It is the case the colleague-ask
	// drafter cannot handle at all.
	through string
	// requester is the rep being introduced, named because a note that does
	// not say who it is about asks for nothing.
	requester string
	// band is how warm the colleague's relationship is, in the vocabulary the
	// page already shows. Carried rather than recomputed here, for the reason
	// org360's introFacts carries it.
	band string
	// lastAt is when the colleague and the contact last spoke, nil when
	// nothing is recorded.
	lastAt *time.Time
	// value is the rep's own sentence on why this is worth the contact's time.
	// Empty is ordinary: the note then asks for a conversation without
	// claiming a reason, which is better than inventing one.
	value string
	lang  textlang.Lang
}

// DraftIntroNote implements POST /people/{id}/intro-note-draft.
func (h Reads) DraftIntroNote(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.DraftIntroNoteJSONRequestBody
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := checkNoteIDs(body); err != nil {
		httperr.Write(w, r, err)
		return
	}

	facts, err := h.noteFactsFor(r.Context(), ids.From[ids.PersonKind](ids.UUID(id)), body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, writeIntroNote(r.Context(), h.introNoteLane, facts))
}

// checkNoteIDs refuses an id the caller did not send, by name.
//
// Its own function because the refusal is the point, and because a gate holds
// it: an absent key decodes to the zero UUID with no error, reaches the route
// lookup, matches nothing, and comes back as "that colleague has no route to
// this contact" — a refusal about a colleague the caller never named and cannot
// connect to anything they did send.
//
// Held by: TestEveryRequiredBodyIDIsNamedWhenAbsent
// (backend/internal/compose/network/intronoteids_test.go)
func checkNoteIDs(body crmcontracts.DraftIntroNoteJSONRequestBody) error {
	if err := httperr.RequireBodyID("via_user_id", ids.UUID(body.ViaUserId)); err != nil {
		return err
	}
	// A null through_person_id means a direct route and is the ordinary case.
	// A present-but-zero one is a client bug, and answering "that colleague has
	// no route to this contact" about the nil UUID would hide it behind a
	// plausible-sounding refusal.
	if body.ThroughPersonId != nil {
		return httperr.RequireBodyID("through_person_id", ids.UUID(*body.ThroughPersonId))
	}
	return nil
}

// noteFactsFor reads the route out of the graph and refuses one that is not
// there.
//
// The graph read is what carries the gates: buildPersonGraph 404s a contact
// this caller cannot see, and every route it returns was built from edges that
// already passed them. So a rep cannot draft a note about a contact they may
// not read, nor claim a colleague relationship no record holds — the refusal
// comes from the same read the page draws itself from.
func (h Reads) noteFactsFor(
	ctx context.Context, personID ids.PersonID, body crmcontracts.DraftIntroNoteJSONRequestBody,
) (noteFacts, error) {
	var facts noteFacts
	err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		graph := crmcontracts.PersonGraph{
			PersonId:      openapi_types.UUID(personID.UUID),
			Nodes:         []crmcontracts.PersonGraphNode{},
			Edges:         []crmcontracts.PersonGraphEdge{},
			GroupsOmitted: []crmcontracts.PersonGraphGroupsOmitted{},
		}
		if err := h.buildPersonGraph(ctx, tx, personID, h.now(), &graph); err != nil {
			return err
		}
		route, ok := routeMatching(&graph, body)
		if !ok {
			// A validation failure rather than a not-found: the contact exists
			// and this caller may read them; what does not exist is the ROUTE
			// they described. A 404 here would send a rep looking for a
			// missing person.
			return httperr.Validation("via_user_id", "no_such_route",
				"that colleague has no recorded route to this contact")
		}
		requester, err := h.callerName(ctx, tx)
		if err != nil {
			return err
		}
		facts = factsFromRoute(&graph, route, requester, body)
		return nil
	})
	if err != nil {
		return noteFacts{}, err
	}
	return facts, nil
}

// routeMatching finds the route the caller described, among the ones the graph
// actually holds.
//
// Matched on BOTH ends: the colleague, and the intermediary where the caller
// named one. Matching on the colleague alone would let a request for a direct
// route be answered with a note written for a hand-off through somebody, which
// describes a different favour than the one being asked for.
func routeMatching(
	graph *crmcontracts.PersonGraph, body crmcontracts.DraftIntroNoteJSONRequestBody,
) (crmcontracts.PersonGraphRouteCandidate, bool) {
	if graph.Routes == nil {
		return crmcontracts.PersonGraphRouteCandidate{}, false
	}
	for _, route := range *graph.Routes {
		if ids.UUID(route.ViaUserId) != ids.UUID(body.ViaUserId) {
			continue
		}
		if !sameIntermediary(route.ThroughPersonId, body.ThroughPersonId) {
			continue
		}
		return route, true
	}
	return crmcontracts.PersonGraphRouteCandidate{}, false
}

// sameIntermediary compares two optional contact ids, where absent means "a
// direct route" on both sides.
func sameIntermediary(onRoute, asked *openapi_types.UUID) bool {
	if onRoute == nil || asked == nil {
		return onRoute == nil && asked == nil
	}
	return ids.UUID(*onRoute) == ids.UUID(*asked)
}

// factsFromRoute reads the note's facts off the route the graph returned.
//
// Nothing here is derived a second time. The band and the last-contact instant
// are read straight off the route the graph returned, which is the same value
// the map draws from — so the note describes the relationship the page shows
// rather than a second reading of it.
func factsFromRoute(
	graph *crmcontracts.PersonGraph,
	route crmcontracts.PersonGraphRouteCandidate,
	requester string,
	body crmcontracts.DraftIntroNoteJSONRequestBody,
) noteFacts {
	facts := noteFacts{
		colleague: route.ViaDisplayName,
		contact:   anchorLabel(graph),
		requester: requester,
		lastAt:    route.Evidence.LastAt,
		// The language the writer defaults, rather than a guess made here.
		lang: textlang.Unknown,
	}
	if route.ThroughDisplayName != nil {
		facts.through = *route.ThroughDisplayName
	}
	if route.StrengthBucket != nil {
		facts.band = string(*route.StrengthBucket)
	}
	if body.ValueForTarget != nil {
		facts.value = *body.ValueForTarget
	}
	return facts
}

// anchorLabel is the contact this graph is about.
func anchorLabel(graph *crmcontracts.PersonGraph) string {
	for i := range graph.Nodes {
		if graph.Nodes[i].Group == crmcontracts.PersonGraphNodeGroupAnchor {
			return graph.Nodes[i].Label
		}
	}
	return ""
}

// callerName is who the note is asking on behalf of.
//
// Read from the authenticated principal's own user row, never from the body: a
// note that could name its own requester would let one rep put words in
// another's mouth, in a message a customer reads.
func (h Reads) callerName(ctx context.Context, tx pgx.Tx) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return "", fmt.Errorf(
			"network: drafting a note needs an authenticated person: %w",
			apperrors.ErrPermissionDenied)
	}
	names, err := UserNames(ctx, tx, []ids.UUID{actor.UserID})
	if err != nil {
		return "", err
	}
	return names[actor.UserID], nil
}
