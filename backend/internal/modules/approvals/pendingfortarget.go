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
// Every row here shares ONE target, so the target-visibility half of
// decidable is asked once for the record rather than once per row — the
// inbox's per-row probe exists because its rows point at different records.
// The per-kind grant check still varies by row and stays in the loop.
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
	visible, err := targetVisible(ctx, tx, &targetType, &targetID)
	if err != nil {
		return nil, err
	}
	if !visible {
		// The record itself is one this caller could not read — an ungranted
		// type or an out-of-scope row — so nothing staged against it is
		// decidable, and saying so is the same existence-hiding answer the
		// record read gives.
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
		out = append(out, wire(a, now))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
