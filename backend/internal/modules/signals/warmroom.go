// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The warm/cold join (B-E08.3, features/07 §9 [MVP]) — the V1-WOW core:
// a signal resolved to a company where we already hold a live
// contact edge is WARM and routes to the warm room; a resolved
// company with no contact is COLD and routes to the cold queue. The
// answer is EVIDENCE — the source signal id, the resolved company id, and the
// specific contact id(s) in our own graph, each with its explainable §4
// strength — never a bare score. The join reads only company-level rows
// and our own relational core; it creates nothing (P11/P12).

package signals

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RouteInEdge pairs the contract evidence row with its contact id for the
// route-in ranking.
type RouteInEdge struct {
	ContactID ids.ContactID
	Contact   crmcontracts.SignalWarmContact
}

// Warmth computes the warm/cold branch for a resolved signal.
func (s *Store) Warmth(ctx context.Context, signalID ids.SignalID, now time.Time) (crmcontracts.SignalWarmth, error) {
	if err := auth.Require(ctx, "signal", principal.ActionRead); err != nil {
		return crmcontracts.SignalWarmth{}, err
	}
	var sig crmcontracts.Signal
	var contacts []RouteInEdge
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureSignalVisible(ctx, tx, signalID.UUID); err != nil {
			return err
		}
		var err error
		if sig, err = readSignal(ctx, tx, signalID, storekit.LiveOnly); err != nil {
			return err
		}
		if sig.ResolutionState != resolutionResolved || sig.ResolvedCompanyId == nil {
			return &NoWarmthError{Reason: fmt.Sprintf(
				"signal is %s: only a signal resolved to a company has a warm/cold branch", sig.ResolutionState)}
		}
		contacts, err = RouteInEdges(ctx, tx, ids.From[ids.CompanyKind](ids.UUID(*sig.ResolvedCompanyId)))
		return err
	})
	if err != nil {
		return crmcontracts.SignalWarmth{}, err
	}

	// Strength rides the injected §4 seam (B-E13.16) — outside the row
	// transaction, exactly like the contacts module's own company roll-up. A
	// contact outside the caller's row scope was already excluded by the
	// edge query; a residual scope miss contributes nothing rather than
	// out-seeing the contact list.
	measured := make(map[ids.ContactID]RelationshipStrength, len(contacts))
	for _, c := range contacts {
		strength, err := s.strength.ContactStrength(ctx, c.ContactID, now)
		switch {
		case errors.Is(err, apperrors.ErrNotFound):
			continue
		case err != nil:
			return crmcontracts.SignalWarmth{}, fmt.Errorf("relationship strength for contact: %w", err)
		}
		measured[c.ContactID] = strength
	}
	scored := RankRouteIn(contacts, func(contactID ids.ContactID) (int, bool) {
		strength, ok := measured[contactID]
		return strength.Strength, ok
	})
	for i := range scored {
		strength := measured[scored[i].ContactID]
		scored[i].Contact.Strength = strength.Strength
		scored[i].Contact.StrengthBucket = crmcontracts.SignalWarmContactStrengthBucket(strength.Bucket)
	}

	out := crmcontracts.SignalWarmth{
		SourceSignalId:    sig.Id,
		ResolvedCompanyId: *sig.ResolvedCompanyId,
		ContactIds:        []openapi_types.UUID{},
		Contacts:          []crmcontracts.SignalWarmContact{},
		Warm:              len(scored) > 0,
		Routing:           crmcontracts.SignalWarmthRouting("cold_queue"),
	}
	if out.Warm {
		out.Routing = crmcontracts.SignalWarmthRouting("warm_room")
	}
	for _, c := range scored {
		out.ContactIds = append(out.ContactIds, openapi_types.UUID(c.ContactID.UUID))
		out.Contacts = append(out.Contacts, c.Contact)
	}
	return out, nil
}

// RankRouteIn orders route-in edges the way the warm room ranks them:
// strongest §4 relationship first, contact id ascending as the deterministic
// tie-break. An edge whose strength the caller could not resolve is DROPPED
// — the warm branch means "we hold a relationship we can measure", and an
// unmeasured contact ranked as a zero would read as a cold route in rather
// than as no answer.
//
// The ranking is a pure function so its two readers cannot disagree: the
// warm/cold join resolves strength through the injected seam outside its
// transaction, while the company view's connection graph resolves it in
// batch inside one — same order, different plumbing.
func RankRouteIn(edges []RouteInEdge, score func(ids.ContactID) (int, bool)) []RouteInEdge {
	scored := make([]RouteInEdge, 0, len(edges))
	strengths := make(map[ids.ContactID]int, len(edges))
	for _, edge := range edges {
		strength, ok := score(edge.ContactID)
		if !ok {
			continue
		}
		strengths[edge.ContactID] = strength
		scored = append(scored, edge)
	}
	sort.SliceStable(scored, func(i, j int) bool {
		left, right := scored[i].ContactID, scored[j].ContactID
		if strengths[left] != strengths[right] {
			return strengths[left] > strengths[right]
		}
		return left.String() < right.String()
	})
	return scored
}

// RouteInEdges finds the live contact edges anchoring the company in OUR
// graph: current employment at the company, or a stakeholder seat on one of
// the company's live deals. Row-scoped — a contact the caller cannot see
// cannot be their evidence. It reads inside the CALLER's transaction, so
// the answer shares the caller's instant.
//
// Exported because the warm/cold join is not its only reader: the company
// view's connection graph marks the same route-in contact, and a second
// spelling of "who anchors this account" would let the card and the warm
// room name different contacts to ask for an intro.
//
// It carries the CONTACT object gate itself. Every row it returns names a
// contact, and the row-scope clause below narrows WHICH contacts a caller sees,
// never whether they may see contacts at all — so a caller holding the signal
// grant and not the contact one is refused here rather than at whichever call
// site remembered to ask.
//
// It carries the EDGE gate for the same reason, and the name says why: these
// ARE edges, returned with their kind and role. Refusing is the honest answer
// on both surfaces — company360 names `intro_path` in groups_omitted, and the warm
// room refuses outright rather than reporting a COLD verdict it reached by not
// being allowed to look for warmth.
func RouteInEdges(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) ([]RouteInEdge, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos := arg(companyID)
	edgeBound, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	if edgeBound != "" {
		edgeBound = " AND " + edgeBound
	}

	scope, err := auth.ScopeClauseFor(ctx, "contact", "p", arg)
	if err != nil {
		return nil, err
	}
	visible := ""
	if scope != "" {
		visible = " AND " + scope
	}

	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT p.id, p.full_name, r.kind, r.role
		FROM relationship r
		JOIN contact p ON p.id = r.contact_id AND p.archived_at IS NULL
		WHERE r.archived_at IS NULL AND r.contact_id IS NOT NULL
		  AND ((r.kind = 'employment' AND r.company_id = $%[1]d
		        AND `+employment.IsCurrentSQL("r.ended_at")+`)
		    OR (r.kind = 'deal_stakeholder' AND r.ended_at IS NULL AND r.deal_id IN (
		          SELECT d.id FROM deal d WHERE d.company_id = $%[1]d AND d.archived_at IS NULL)))%s%s
		ORDER BY p.id, r.kind`, companyPos, edgeBound, visible), args...)
	if err != nil {
		return nil, fmt.Errorf("contact edges: %w", err)
	}
	defer rows.Close()

	// One evidence row per contact: employment is the primary edge when a
	// contact holds both (it is the durable "we know someone there" fact).
	byContact := map[ids.ContactID]RouteInEdge{}
	var order []ids.ContactID
	for rows.Next() {
		var contactID ids.ContactID
		var fullName, role *string
		var kind string
		if err := rows.Scan(&contactID, &fullName, &kind, &role); err != nil {
			return nil, err
		}
		if have, ok := byContact[contactID]; ok {
			if have.Contact.RelationshipKind == "employment" || kind != "employment" {
				continue
			}
		} else {
			order = append(order, contactID)
		}
		byContact[contactID] = RouteInEdge{
			ContactID: contactID,
			Contact: crmcontracts.SignalWarmContact{
				ContactId:        openapi_types.UUID(contactID.UUID),
				FullName:         fullName,
				RelationshipKind: crmcontracts.SignalWarmContactRelationshipKind(kind),
				RelationshipRole: role,
			},
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]RouteInEdge, 0, len(order))
	for _, id := range order {
		out = append(out, byContact[id])
	}
	return out, nil
}
