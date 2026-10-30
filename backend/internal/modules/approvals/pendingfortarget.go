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

// PendingForTarget uses the inbox's full predicate: visibility depends on the
// proposed action as well as the record, and evidence can narrow it further.
// Explicit record review includes personal mutes; the ordinary queue filters them.
func (s *Service) PendingForTarget(ctx context.Context, tx pgx.Tx, targetType string, targetID ids.UUID, limit int) ([]crmcontracts.Approval, error) {
	if err := actingForAHuman(ctx); err != nil {
		return nil, err
	}
	p, _ := principal.Actor(ctx)
	if limit <= 0 || limit > PendingScanCap {
		limit = PendingScanCap
	}
	now := s.now()
	in := targetPending(targetType, targetID)
	q, args := approvalPageQuery(in, nil)
	batch, err := collect(ctx, tx, q, args)
	if err != nil {
		return nil, err
	}
	// Lazy expiry: a row past its expiry is not a decision anyone still owes,
	// so it must not appear as one on the record page.
	visible, err := pendingEligible(ctx, tx, p, in, now)(batch)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.Approval, 0, len(batch))
	for i, a := range batch {
		if !visible[i] {
			continue
		}
		out = append(out, wire(a, now))
		if len(out) >= limit {
			break
		}
	}
	if err := s.assignmentStates(ctx, tx, out, commitmentAssignees(out)); err != nil {
		return nil, err
	}
	withoutOwnersRecords(ctx, out)
	return out, nil
}

// PendingOnTarget is one record's decidable proposals: the first PendingScanCap
// of them, and how many there are in all.
type PendingOnTarget struct {
	Approvals []crmcontracts.Approval
	Total     int
}

// PendingForTargetCounted lists what PendingForTarget lists and counts every
// row past its cap in the same walk. A record with one page of proposals costs
// one page read.
func (s *Service) PendingForTargetCounted(ctx context.Context, tx pgx.Tx, targetType string, targetID ids.UUID) (PendingOnTarget, error) {
	if err := actingForAHuman(ctx); err != nil {
		return PendingOnTarget{}, err
	}
	p, _ := principal.Actor(ctx)
	now := s.now()
	in := targetPending(targetType, targetID)
	eligible := pendingEligible(ctx, tx, p, in, now)
	var out PendingOnTarget
	err := walkInbox(ctx, tx, in, nil, func(batch []row) (bool, error) {
		visible, err := eligible(batch)
		if err != nil {
			return false, err
		}
		for i, a := range batch {
			if !visible[i] {
				continue
			}
			out.Total++
			if len(out.Approvals) < PendingScanCap {
				out.Approvals = append(out.Approvals, wire(a, now))
			}
		}
		return false, nil
	})
	if err != nil {
		return PendingOnTarget{}, err
	}
	if err := s.assignmentStates(ctx, tx, out.Approvals, commitmentAssignees(out.Approvals)); err != nil {
		return PendingOnTarget{}, err
	}
	withoutOwnersRecords(ctx, out.Approvals)
	return out, nil
}

// targetPending selects the stored-pending rows standing against one record.
// It names no requested decider and binds no mutes, so the record page lists
// and counts every proposal the caller could decide there.
func targetPending(targetType string, targetID ids.UUID) ListInput {
	pending := statusPending
	return ListInput{Status: &pending, TargetType: &targetType, TargetID: &targetID}
}
