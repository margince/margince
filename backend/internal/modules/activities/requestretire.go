// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// agedReminderBatch bounds one pass, like the minting pass beside it.
const agedReminderBatch = 64

// retireAgedReminders archives the reminders this pass filed whose request has
// passed the waiting horizon, unless a human worked them. Archiving rather than
// completing: an aged request is not an answered one. heldRequestSQL is the
// other side of the same rule: a reminder a human wrote keeps its request.
func (s *Store) retireAgedReminders(ctx context.Context, tx pgx.Tx, asOf time.Time) error {
	horizon, err := s.waitingHorizonFor(ctx, tx, asOf)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT reminder.id FROM activity reminder
		  JOIN activity request ON request.id = reminder.source_activity_id
		 WHERE reminder.source_system = $1
		   AND NOT reminder.is_done AND reminder.archived_at IS NULL
		   AND reminder.captured_by = $2
		   AND request.occurred_at < $3::timestamptz - make_interval(days => $4)
		   AND NOT `+humanWroteReminderSQL("reminder")+`
		 ORDER BY request.occurred_at, reminder.id
		 LIMIT $5
		 FOR UPDATE OF reminder SKIP LOCKED`,
		EmailRequestTaskSource, OwedVerdictCapturedBy, asOf, horizon, agedReminderBatch)
	if err != nil {
		return fmt.Errorf("activities: reading reminders past the waiting horizon: %w", err)
	}
	aged, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("activities: reading reminders past the waiting horizon: %w", err)
	}
	for _, id := range aged {
		if _, err := archiveActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), nil); err != nil {
			return fmt.Errorf("activities: retiring the aged reminder %s: %w", id, err)
		}
	}
	return nil
}
