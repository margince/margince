// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AnswerDueTx writes the due date a human chose when asked about a captured
// task's date: nil clears it. A human who kept a date outside the window sets
// confirmed, which admits it; any other date meets the window.
//
// The task is locked first and must still be live, held by owner and undated.
// Otherwise the question is stale and nothing is written (false).
func (s *Store) AnswerDueTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, owner ids.UUID, due *time.Time, confirmed bool) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	var assignee *ids.UUID
	var current *time.Time
	err := tx.QueryRow(ctx, `SELECT assignee_id, due_at FROM activity WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, id).Scan(&assignee, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if current != nil || assignee == nil || *assignee != owner {
		return false, nil
	}
	at := s.now()
	in := UpdateActivityInput{DueAt: due, confirmedDue: confirmed, recordedAt: &at, recordZone: s.recordZone}
	if due == nil {
		return true, nil
	}
	if err := s.readPatchColumns(ctx, tx, &in); err != nil {
		return false, err
	}
	_, err = updateActivityInTx(ctx, tx, id, in)
	return err == nil, err
}
