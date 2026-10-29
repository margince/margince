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
	pending := statusPending
	q, args := approvalPageQuery(ListInput{
		Status: &pending, TargetType: &targetType, TargetID: &targetID,
	}, nil)
	batch, err := collect(ctx, tx, q, args)
	if err != nil {
		return nil, err
	}
	// Lazy expiry: a row past its expiry is not a decision anyone still owes,
	// so it must not appear as one on the record page.
	visible, err := askWhere(batch, func(a row) bool { return a.PendingAt(now) },
		func(rows []row) ([]bool, error) { return decidableRows(ctx, tx, p, rows) })
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
