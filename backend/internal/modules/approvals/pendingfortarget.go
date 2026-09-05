// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The proposals standing against ONE record, for the panel a record page shows.
//
// A different question from the inbox's, and gated differently for it: the
// record's own visibility is settled once by the caller, where the inbox has to
// probe per row because its rows point at different records.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PendingForTarget returns the pending approvals staged against ONE record,
// filtered by the same decidability rule the inbox uses — a record page must
// never become the side channel List refuses to be. It takes the caller's
// transaction so a composite record read assembles every section at one
// instant instead of opening a second connection for this one.
//
// It answers the wire shape rather than the store row: the caller is another
// package, and re-deriving the effective status (lazy expiry) outside this
// module is exactly the drift the type keeps unexported to prevent.
//
// It IS decidable, evaluated in the order this shape allows rather than by a
// second rule. Every row shares ONE target, so decidable's target half is asked
// once for the record instead of once per row — the inbox's per-row probe
// exists because its rows point at different records — and the two halves that
// vary by row stay in the loop: the per-kind grant check, and the self-only
// narrowing (withheldFromOtherSeats), which compares the caller against the
// seat each ROW was staged for and not against the target.
//
// The hoisted half is targetDecidable, the WRITE one, and not targetVisible.
// They differ in exactly the arms a manual grant can widen, so reading-scope
// was enough to put a pending proposal on the panel — and the panel renders the
// proposal, summary and proposed change included, not merely the fact that one
// exists. A read-scoped share therefore received a staged change the inbox
// withholds from the same seat.
//
// decidable's own doc is why that cannot stand: it backs List, Get and Decide
// alike so triage visibility and the decision gate can never drift apart. A
// record page answering differently is that drift, wearing a fourth caller.
//
// The scan is bounded at PendingScanCap rows so one record page cannot pay
// for an unbounded backlog. A caller counting rather than listing must treat
// a full result as "this many or more"; limit bounds the RETURNED rows, so
// pass PendingScanCap to mean "everything the scan found".
func (s *Service) PendingForTarget(ctx context.Context, tx pgx.Tx, targetType string, targetID ids.UUID, limit int) ([]crmcontracts.Approval, error) {
	if err := actingForAHuman(ctx); err != nil {
		return nil, err
	}
	p, _ := principal.Actor(ctx)
	if limit <= 0 || limit > PendingScanCap {
		limit = PendingScanCap
	}
	decidableTarget, err := targetDecidable(ctx, tx, &targetType, &targetID)
	if err != nil {
		return nil, err
	}
	if !decidableTarget {
		// The record is one this caller could not decide against — an ungranted
		// type, an out-of-scope row, or a share that only ever let them read —
		// so nothing staged against it is decidable, and saying so is the same
		// existence-hiding answer the record read gives.
		return []crmcontracts.Approval{}, nil
	}
	now := s.now()
	pending := statusPending
	q, args := approvalPageQuery(ListInput{
		Status: &pending, TargetType: &targetType, TargetID: &targetID,
	}, nil)
	batch, err := collect(ctx, tx, q, args)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.Approval, 0, len(batch))
	for i := range batch {
		a := batch[i]
		if a.effectiveStatus(now) != statusPending {
			// Lazy expiry: a row past its expiry is not a decision anyone
			// still owes, so it must not appear as one on the record page.
			continue
		}
		if requireDecisionGrants(p, a) != nil {
			continue
		}
		if withheldFromOtherSeats(p, a) {
			// A self-only staging is one seat's own business, so it is absent
			// from a colleague's record page for the same reason it is absent
			// from their inbox — the panel is the same side channel List
			// refuses to be.
			continue
		}
		out = append(out, wire(a, now))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
