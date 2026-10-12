// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// The note a colleague FORWARDS, as distinct from the ask they receive.
//
// Two messages exist in one workflow and they are written for different
// readers. company360's draftIntroRequest writes the internal ask — its own prompt
// says "the message is TO the colleague" — and this writes the prospect-facing
// note that colleague pastes into the mail they send onward. Collapsing them
// would put an internal favour-ask in front of a customer.
//
// It lives in package network because the facts are the graph's: who the
// colleague is, how warm the relationship is, when they last spoke, and whether
// the route runs through somebody. buildContactGraph is a private method on
// Reads over unexported fields, so a drafter anywhere else would have to
// re-derive all of that — a second answer to "how well do these two know each
// other", free to disagree with the map on screen.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/auth"
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
//
// It binds no Voice DNA, and that is a decision rather than the gap it looks
// like. Every other outbound drafting surface writes as the contact operating
// it, so it loads that colleague's voice; this note is sent by the COLLEAGUE, and
// the rep operating the page is the colleague being introduced. Loading the
// caller's profile here would sign the colleague's message in somebody else's
// style — and the colleague need not be a Margince user at all, so there is
// frequently no profile to load even in principle.
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
	// their voice: they are the sender, and the rep is the colleague being
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
	// company360's introFacts carries it.
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

// DraftIntroNote implements POST /contacts/{id}/intro-note-draft.
func (h Reads) DraftIntroNote(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.DraftIntroNoteJSONRequestBody
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := checkNoteIDs(body); err != nil {
		httperr.Write(w, r, err)
		return
	}

	contactID := ids.From[ids.ContactKind](ids.UUID(id))
	facts, err := h.noteFactsFor(r.Context(), contactID, body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The contact the note is addressed to. Everything the note says is what
	// this product knows about them, so an erasure reaching that contact must
	// reach this call's payloads.
	ctx := ai.WithSubject(r.Context(), contactID.Ref(), facts.contact)
	httperr.WriteJSON(w, http.StatusOK, writeIntroNote(ctx, h.introNoteLane, facts))
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
	// A null through_contact_id means a direct route and is the ordinary case.
	// A present-but-zero one is a client bug, and answering "that colleague has
	// no route to this contact" about the nil UUID would hide it behind a
	// plausible-sounding refusal.
	if body.ThroughContactId != nil {
		return httperr.RequireBodyID("through_contact_id", ids.UUID(*body.ThroughContactId))
	}
	return nil
}

// noteFactsFor reads the route out of the graph and refuses one that is not
// there.
//
// The graph read is what carries the gates: buildContactGraph 404s a contact
// this caller cannot see, and every route it returns was built from edges that
// already passed them. So a rep cannot draft a note about a contact they may
// not read, nor claim a colleague relationship no record holds — the refusal
// comes from the same read the page draws itself from.
func (h Reads) noteFactsFor(
	ctx context.Context, contactID ids.ContactID, body crmcontracts.DraftIntroNoteJSONRequestBody,
) (noteFacts, error) {
	var facts noteFacts
	err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		graph := crmcontracts.ContactGraph{
			ContactId:     openapi_types.UUID(contactID.UUID),
			Nodes:         []crmcontracts.ContactGraphNode{},
			Edges:         []crmcontracts.ContactGraphEdge{},
			GroupsOmitted: []crmcontracts.ContactGraphGroupsOmitted{},
		}
		if err := h.buildContactGraph(ctx, tx, contactID, h.now(), &graph); err != nil {
			return err
		}
		route, ok := routeMatching(&graph, body)
		if !ok {
			// A validation failure rather than a not-found: the contact exists
			// and this caller may read them; what does not exist is the ROUTE
			// they described. A 404 here would send a rep looking for a
			// missing contact.
			return httperr.Validation("via_user_id", "no_such_route",
				"that colleague has no recorded route to this contact")
		}
		requester, err := h.callerName(ctx, tx)
		if err != nil {
			return err
		}
		facts = factsFromRoute(&graph, route, requester, body)
		// The CORRESPONDENCE decides the language, not the names: "Brandt GmbH"
		// detects as nothing, so a German customer was being written to in
		// English. Read here, where the transaction is, and detected by the one
		// detector the sibling intro uses (company360/introdraft.go).
		said, err := contactCorrespondence(ctx, tx, contactID, h.now().UTC())
		if err != nil {
			return err
		}
		facts.lang = textlang.Detect(said)
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
	graph *crmcontracts.ContactGraph, body crmcontracts.DraftIntroNoteJSONRequestBody,
) (crmcontracts.ContactGraphRouteCandidate, bool) {
	if graph.Routes == nil {
		return crmcontracts.ContactGraphRouteCandidate{}, false
	}
	for _, route := range *graph.Routes {
		if ids.UUID(route.ViaUserId) != ids.UUID(body.ViaUserId) {
			continue
		}
		if !sameIntermediary(route.ThroughContactId, body.ThroughContactId) {
			continue
		}
		return route, true
	}
	return crmcontracts.ContactGraphRouteCandidate{}, false
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
	graph *crmcontracts.ContactGraph,
	route crmcontracts.ContactGraphRouteCandidate,
	requester string,
	body crmcontracts.DraftIntroNoteJSONRequestBody,
) noteFacts {
	facts := noteFacts{
		colleague: route.ViaDisplayName,
		contact:   anchorLabel(graph),
		requester: requester,
		lastAt:    route.Evidence.LastAt,
		// The language is detected by the caller, which has the transaction to
		// read the correspondence with; this assembler stays pure.
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

// How far back and how much to read before detecting the language. Matched to
// the sibling's window (company360's proposalWindowDays / proposalMessages),
// because both ask what this contact has written lately, and two answers to that
// would make one surface write in German where the other writes in English.
const (
	noteLangWindowDays = 365
	noteLangMessages   = 6
)

// contactCorrespondence is the recent inbound mail from this contact that the
// calling rep may read, as one block of text for language detection.
//
// Bounded at both ends. A future-dated inbound row, from a provider stamp nobody
// validated or a clock askew, sorts first and would spend the sample on mail
// that has not arrived, detecting the language of whatever that row holds.
//
// Gated by auth.ActivityContentClause, the same predicate every other mail read
// goes through, and not softened when it denies. A rep who may read none of this
// contact's mail gets an empty string, which detects as Unknown and is reported
// as "could not determine" rather than answered in English without saying so.
// The two causes have different fixes, one a permission and one a contact who
// has written nothing.
func contactCorrespondence(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, now time.Time,
) (string, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		// Not an error to the caller: the note is still writable, in the
		// default language, and the response says the language was not
		// determined. Refusing the whole draft over a language hint would be a
		// worse answer than writing it in English.
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return "", nil
		}
		return "", err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contactPos := arg(contactID)
	sincePos := arg(now.AddDate(0, 0, -noteLangWindowDays))
	untilPos := arg(now)
	capPos := arg(noteLangMessages)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		scope = "TRUE"
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT coalesce(a.subject, ''), coalesce(a.body, '')
		  FROM activity a
		  JOIN activity_participant ap
		    ON ap.activity_id = a.id AND ap.role = 'from' AND ap.contact_id = $%d
		 WHERE a.direction = 'inbound' AND a.archived_at IS NULL
		   AND a.occurred_at >= $%d AND a.occurred_at <= $%d AND (%s)
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT $%d`, contactPos, sincePos, untilPos, scope, capPos), args...)
	if err != nil {
		return "", fmt.Errorf("network: reading what the contact wrote: %w", err)
	}
	defer rows.Close()
	var text strings.Builder
	for rows.Next() {
		var subject, body string
		if err := rows.Scan(&subject, &body); err != nil {
			return "", err
		}
		text.WriteString(subject + "\n" + body + "\n\n")
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("network: reading what the contact wrote: %w", err)
	}
	return text.String(), nil
}

// anchorLabel is the contact this graph is about.
func anchorLabel(graph *crmcontracts.ContactGraph) string {
	for i := range graph.Nodes {
		if graph.Nodes[i].Group == crmcontracts.ContactGraphNodeGroupAnchor {
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
			"network: drafting a note needs an authenticated contact: %w",
			apperrors.ErrPermissionDenied)
	}
	names, err := UserNames(ctx, tx, []ids.UUID{actor.UserID})
	if err != nil {
		return "", err
	}
	return names[actor.UserID], nil
}
