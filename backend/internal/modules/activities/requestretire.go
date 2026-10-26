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
	cutoff, err := requestReminderCutoff(ctx, tx, asOf)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT reminder.id FROM activity reminder
		  JOIN activity request ON request.id = reminder.source_activity_id
		 WHERE reminder.source_system = @source
		   AND NOT reminder.is_done AND reminder.archived_at IS NULL
		   AND reminder.captured_by = @actor
		   AND request.occurred_at < @cutoff -- occurred_at raw: the age of a captured mail, an instant
		   AND `+pristineRepairTaskSQL("reminder")+`
		 ORDER BY request.occurred_at, reminder.id
		 LIMIT @limit
		 FOR UPDATE OF reminder SKIP LOCKED`,
		pgx.NamedArgs{paramRepairSource: EmailRequestTaskSource, "actor": OwedVerdictCapturedBy, "cutoff": cutoff, "limit": agedReminderBatch})
	if err != nil {
		return fmt.Errorf("activities: reading reminders past the waiting horizon: %w", err)
	}
	locked, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("activities: reading reminders past the waiting horizon: %w", err)
	}
	// Asked again now the rows are locked: the read above judged the audit
	// trail as of its own snapshot, so a human edit that committed while it
	// waited for a lock is visible only to a later statement.
	rows, err = tx.Query(ctx, `SELECT reminder.id FROM activity reminder
		 WHERE reminder.id = ANY(@locked) AND `+pristineRepairTaskSQL("reminder"), pgx.NamedArgs{"locked": locked})
	if err != nil {
		return fmt.Errorf("activities: rechecking aged reminders under their lock: %w", err)
	}
	aged, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("activities: rechecking aged reminders under their lock: %w", err)
	}
	for _, id := range aged {
		if _, err := archiveActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), nil, archiveOptions{}); err != nil {
			return fmt.Errorf("activities: retiring the aged reminder %s: %w", id, err)
		}
	}
	return nil
}
