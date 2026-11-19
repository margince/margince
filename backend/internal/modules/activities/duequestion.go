// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AnswerDueTx writes the due date a human chose when asked about a captured
// task's date: nil clears it. A human who kept a date outside the window sets
// confirmed, which admits it; any other date meets the window.
func (s *Store) AnswerDueTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, due *time.Time, confirmed bool) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	at := s.now()
	in := UpdateActivityInput{DueAt: due, confirmedDue: confirmed, recordedAt: &at, recordZone: s.recordZone}
	if due == nil {
		in.Clear = []string{fieldDueAt}
	}
	if err := s.readPatchColumns(ctx, &in); err != nil {
		return err
	}
	_, err := updateActivityInTx(ctx, tx, id, in)
	return err
}
