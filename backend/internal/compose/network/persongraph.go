// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// The local graph around one contact (ADR-0078 / PO-EXT-3).
//
// It answers ONE question — who here can open a door to this person, and what
// is the evidence they really know them. A diagram of the whole network
// answers nothing; this one is shaped by the question and stops where the
// question stops.
//
// Two groups, and the second exists because the first is so often empty.
// `direct` is the colleagues who have corresponded with the contact
// themselves. `account` is the other contacts at their employer and which
// colleague is warmest with each — the route when nobody here knows the person
// but somebody knows the person sitting next to them.
//
// Row scope is applied PER GROUP rather than once at the root. A root-only
// check would let the account arm name contacts the caller may not read, which
// is the leak this shape exists to avoid: the graph would disclose by
// adjacency exactly what the record list withholds.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The caps. Both are reading limits: past ten colleagues or a dozen
// colleagues-at-the-company nobody is choosing a route any more, they are
// reading an org chart.
const (
	graphDirectCap  = 10
	graphAccountCap = 12
	// Over-fetch before ranking, for the reason SortByStrength documents: the
	// projection is ordered by recency, so capping in SQL would evict a
	// genuinely warmer colleague in favour of a recent one-line reply.
	graphDirectFetch = 100
	// receiptsPerEdge bounds the proof shown for one relationship. Three
	// messages settle "do they actually know them"; a fourth is an archive.
	receiptsPerEdge = 3
)

// GetPersonGraph implements GET /people/{id}/graph.
func (h Reads) GetPersonGraph(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	personID := ids.From[ids.PersonKind](ids.UUID(id))
	now := h.now()
	out := crmcontracts.PersonGraph{
		PersonId:      id,
		Nodes:         []crmcontracts.PersonGraphNode{},
		Edges:         []crmcontracts.PersonGraphEdge{},
		GroupsOmitted: []crmcontracts.PersonGraphGroupsOmitted{},
	}

	ctx := r.Context()
	err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		return h.buildPersonGraph(ctx, tx, personID, now, &out)
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// After the graph's transaction, not inside it: the introductions reader
	// takes a connection of its own, and holding two per request is how a read
	// path starves the pool under load. The routes are stamped on the way out
	// and no earlier read depends on them.
	if err := h.markAskedRoutes(ctx, personID, &out); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// markAskedRoutes greys out the routes the server would refuse.
//
// Two failures leave the graph standing with its routes unstamped, and both
// describe a caller who has already been served a valid picture:
//
//   - A denial. The caller holds no introduction grant, so they have no ask to
//     collide with, and the one they cannot make is refused where they make it.
//   - A not-found. This read gates the contact a SECOND time, in its own
//     transaction, and the graph's own gate already admitted them — so the only
//     way to reach it is a contact archived between the two. The routes are
//     then merely unstamped, where failing would turn a served graph into a 404
//     over a decoration.
//
// Any other fault fails the read, for the reason a missing group does: a graph
// that quietly claims every door is open is worse than an error, because the
// rep acts on it.
func (h Reads) markAskedRoutes(
	ctx context.Context, personID ids.PersonID, out *crmcontracts.PersonGraph,
) error {
	if h.askedRoutes == nil || out.Routes == nil {
		return nil
	}
	asked, err := h.askedRoutes.RouteStates(ctx, personID)
	if isDenied(err) || errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	stampAvailability(*out.Routes, asked)
	return nil
}

// buildPersonGraph assembles both groups inside one transaction, so the
// picture describes one moment rather than a stack of independently-timed
// reads.
func (h Reads) buildPersonGraph(
	ctx context.Context,
	tx pgx.Tx,
	personID ids.PersonID,
	now time.Time,
	out *crmcontracts.PersonGraph,
) error {
	// The anchor read is mandatory and its refusal is the whole read's
	// refusal: a graph around a contact the caller cannot see must 404 rather
	// than return an empty picture, which would confirm the record exists.
	anchor, err := anchorNode(ctx, tx, personID)
	if err != nil {
		return err
	}
	out.Nodes = append(out.Nodes, anchor)

	dropped := struct {
		Account *int `json:"account,omitempty"`
		Direct  *int `json:"direct,omitempty"`
		Peer    *int `json:"peer,omitempty"`
	}{}

	directDropped, err := h.addDirectGroup(ctx, tx, personID, now, out)
	switch {
	case err == nil:
		dropped.Direct = &directDropped
	case isDenied(err):
		out.GroupsOmitted = append(out.GroupsOmitted, crmcontracts.PersonGraphGroupsOmittedDirect)
	default:
		return err
	}

	accountDropped, err := h.addAccountGroup(ctx, tx, personID, now, out)
	switch {
	case err == nil:
		dropped.Account = &accountDropped
	case isDenied(err):
		out.GroupsOmitted = append(out.GroupsOmitted, crmcontracts.PersonGraphGroupsOmittedAccount)
	default:
		return err
	}

	// After the account arm on purpose: a peer already drawn as an account
	// contact keeps that node, and the peer edge hangs off it. No denial
	// branch — this arm needs only the person read the anchor already
	// required, so a caller refused there never reaches it.
	peerDropped, err := h.addPeerGroup(ctx, tx, personID, now, out)
	if err != nil {
		return err
	}
	dropped.Peer = &peerDropped

	out.DroppedCount = &dropped
	routes := chooseRoutes(out, now)
	out.Routes = &routes
	out.Route = chooseRoute(routes)
	return nil
}

// isDenied reports the one failure a group survives: the caller lacks its
// grant, so the group is named as withheld rather than returned empty. Any
// other fault fails the read, because half a graph is worse than an error —
// the reader cannot tell which half is missing.
func isDenied(err error) bool {
	return err != nil && errors.Is(err, apperrors.ErrPermissionDenied)
}

// anchorNode reads the contact this graph is about.
func anchorNode(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (crmcontracts.PersonGraphNode, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return crmcontracts.PersonGraphNode{}, err
	}
	if err := auth.EnsureVisibleLive(ctx, tx, "person", personID.UUID); err != nil {
		return crmcontracts.PersonGraphNode{}, err
	}
	var name string
	var title *string
	if err := tx.QueryRow(ctx,
		`SELECT full_name, title FROM person WHERE id = $1`, personID).Scan(&name, &title); err != nil {
		return crmcontracts.PersonGraphNode{}, fmt.Errorf("network: reading the contact a graph is about: %w", err)
	}
	pid := openapi_types.UUID(personID.UUID)
	return crmcontracts.PersonGraphNode{
		Id:       personNodeID(personID.UUID),
		Type:     crmcontracts.PersonGraphNodeTypeContact,
		Group:    crmcontracts.PersonGraphNodeGroupAnchor,
		Label:    name,
		Sublabel: title,
		PersonId: &pid,
	}, nil
}

// addDirectGroup adds the colleagues who have corresponded with this contact,
// warmest first, each edge carrying the messages behind it.
func (h Reads) addDirectGroup(
	ctx context.Context,
	tx pgx.Tx,
	personID ids.PersonID,
	now time.Time,
	out *crmcontracts.PersonGraph,
) (int, error) {
	edges, err := search.EdgesForPerson(ctx, tx, personID.UUID, graphDirectFetch)
	if err != nil {
		return 0, err
	}
	search.SortByStrength(edges, now)
	// The remainder counts from the FETCH, and the fetch is itself bounded —
	// so on a contact with more than graphDirectFetch colleagues the number
	// understates. That bound is stated rather than hidden: a workspace where
	// a hundred people have corresponded with one contact is far outside the
	// shape this card is for, and over-fetching the whole set to make the
	// remainder exact would cost every ordinary read to make one pathological
	// read honest.
	dropped := 0
	if len(edges) > graphDirectCap {
		dropped = len(edges) - graphDirectCap
		edges = edges[:graphDirectCap]
	}
	names, err := UserNames(ctx, tx, EdgeUsers(edges))
	if err != nil {
		return 0, err
	}
	for _, e := range edges {
		out.Nodes = append(out.Nodes, colleagueNode(e.UserID, names[e.UserID], crmcontracts.PersonGraphNodeGroupDirect))
		receipts, err := edgeReceipts(ctx, tx, e.UserID, e.PersonID)
		if err != nil {
			return 0, err
		}
		edge := wireEdge(e, userNodeID(e.UserID), personNodeID(e.PersonID), now)
		edge.Receipts = &receipts
		out.Edges = append(out.Edges, edge)
	}
	return dropped, nil
}

// edgeReceipts names up to three messages behind one colleague-contact pair.
//
// Each candidate passes the caller's own activity row scope INSIDE the query.
// The edge's counts are pooled metadata and disclosable; the messages are
// correspondence and are not, so an activity outside the caller's scope is
// absent from the receipts while still counted in the total. That difference
// is the whole reason receipts are a separate read rather than a join.
func edgeReceipts(ctx context.Context, tx pgx.Tx, userID, personID ids.UUID) ([]crmcontracts.PersonGraphReceipt, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		// No activity grant is not a reason to fail the graph: the edge's
		// counts stand on the person grant alone, and the receipts are the
		// part this caller may not have.
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return []crmcontracts.PersonGraphReceipt{}, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	userPos, personPos := arg(userID), arg(personID)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "true"
	}
	limitPos := arg(receiptsPerEdge)

	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT a.id, a.subject, a.occurred_at, a.kind
		  FROM activity a
		 WHERE a.archived_at IS NULL
		   AND EXISTS (SELECT 1 FROM activity_participant up
		                WHERE up.activity_id = a.id AND up.user_id = $%d)
		   AND EXISTS (SELECT 1 FROM activity_participant pp
		                WHERE pp.activity_id = a.id AND pp.person_id = $%d)
		   AND (%s)
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT $%d`, userPos, personPos, scope, limitPos), args...)
	if err != nil {
		return nil, fmt.Errorf("network: reading the messages behind a relationship: %w", err)
	}
	defer rows.Close()

	out := []crmcontracts.PersonGraphReceipt{}
	for rows.Next() {
		var r crmcontracts.PersonGraphReceipt
		var activityID ids.UUID
		var kind string
		if err := rows.Scan(&activityID, &r.Subject, &r.OccurredAt, &kind); err != nil {
			return nil, fmt.Errorf("network: reading a message behind a relationship: %w", err)
		}
		r.ActivityId = openapi_types.UUID(activityID)
		r.Kind = crmcontracts.PersonGraphReceiptKind(kind)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("network: reading the messages behind a relationship: %w", err)
	}
	return out, nil
}

// chooseRoute picks the warmest way in.
//
// Deterministic, and the preference order is the whole recommendation: a
// direct relationship beats an indirect one however warm the indirect one
// looks, because "she already knows you" is a different kind of claim from
// "he knows someone at her company".
//
// It reads the head of the candidate list rather than ranking the edges a
// second time. Two rankings over one set of facts are two answers to one
// question, and the day they disagree the card recommends one colleague and
// lists a different one first.
func chooseRoute(candidates []crmcontracts.PersonGraphRouteCandidate) *crmcontracts.PersonGraphRoute {
	if len(candidates) == 0 {
		return nil
	}
	best := candidates[0]
	route := crmcontracts.PersonGraphRoute{
		ViaUserId:      best.ViaUserId,
		ViaDisplayName: best.ViaDisplayName,
		Why:            proofLineFor(best.Evidence),
	}
	if best.RouteType == crmcontracts.PersonGraphRouteTypeThroughContact {
		route.ThroughPersonId = best.ThroughPersonId
		route.ThroughDisplayName = best.ThroughDisplayName
	}
	return &route
}

// proofLineFor writes the route's evidence as a sentence.
//
// English, and the contract says so: `why` is the fallback for a caller that
// has not adopted the structured `evidence`, which is what the product renders
// in the reader's own language. A new surface reads the facts, not this.
func proofLineFor(ev crmcontracts.PersonGraphRouteEvidence) string {
	when := "no recent contact"
	if ev.DaysSinceLast != nil {
		switch days := *ev.DaysSinceLast; days {
		case 0:
			when = "last contact today"
		case 1:
			when = "last contact yesterday"
		default:
			when = fmt.Sprintf("last contact %d days ago", days)
		}
	}
	if ev.TwoWay {
		return fmt.Sprintf("%s in 90 days · %s", plural(ev.Interactions90d, "two-way exchange"), when)
	}
	return fmt.Sprintf("%s in 90 days, one-sided · %s", plural(ev.Interactions90d, "interaction"), when)
}

// plural writes a count and its noun. The proof line is read by a person
// deciding whether to ask a colleague for a favour, and "1 interactions"
// undermines the claim it is making.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// wireEdge renders one interaction edge, scoring it through the same §4 curve
// every other surface uses.
func wireEdge(e search.InteractionEdge, from, to string, now time.Time) crmcontracts.PersonGraphEdge {
	score := e.StrengthOf(now)
	in, outCount := e.InCount90d, e.OutCount90d
	last := e.LastAt
	return crmcontracts.PersonGraphEdge{
		From:            from,
		To:              to,
		StrengthBucket:  crmcontracts.PersonGraphEdgeStrengthBucket(score.Bucket),
		Interactions90d: e.Count90d,
		Inbound90d:      &in,
		Outbound90d:     &outCount,
		LastAt:          &last,
	}
}

func colleagueNode(userID ids.UUID, name string, group crmcontracts.PersonGraphNodeGroup) crmcontracts.PersonGraphNode {
	uid := openapi_types.UUID(userID)
	return crmcontracts.PersonGraphNode{
		Id:     userNodeID(userID),
		Type:   crmcontracts.PersonGraphNodeTypeColleague,
		Group:  group,
		Label:  name,
		UserId: &uid,
	}
}

// Node ids are prefixed rather than bare uuids: a user and a person are
// different kinds of node, and an edge that referred to a bare id would be
// ambiguous the first time the two id spaces collided.
func userNodeID(id ids.UUID) string   { return "user:" + id.String() }
func personNodeID(id ids.UUID) string { return "person:" + id.String() }
