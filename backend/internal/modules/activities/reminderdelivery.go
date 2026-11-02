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

// ClaimReminderDelivery answers whether the caller is the one to show a due
// reminder's browser notification. The first claim at a reminder instant takes
// it, from any browser. A moved remind_at or a new assignee differs from the
// claim, so the reminder is armed again.
//
// Only a row the caller's reminder_due list returns at that instant can be
// claimed, asked of the list itself. Any other id answers false, which says
// nothing about a task the caller cannot see. The record is about a
// notification, not the task, so it has no audit row.
func (s *Store) ClaimReminderDelivery(ctx context.Context, id ids.ActivityID, remindAt time.Time) (bool, error) {
	due, _, err := s.ListActivities(ctx, ListActivitiesInput{RemindersDue: true, onlyID: &id})
	if err != nil || len(due) != 1 || due[0].RemindAt == nil || !due[0].RemindAt.Equal(remindAt) {
		return false, err
	}
	return s.recordReminderClaim(ctx, id, remindAt)
}

// recordReminderClaim locks the task and re-reads what can change after the
// list read. A completion, a moved reminder or a new assignee that commits
// first leaves nothing to claim.
func (s *Store) recordReminderClaim(ctx context.Context, id ids.ActivityID, remindAt time.Time) (bool, error) {
	var claimed bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO activity_reminder_delivery (activity_id, delivered_for, delivered_to)
			SELECT id, remind_at, assignee_id FROM activity
			 WHERE activity_thread_available(thread_key) AND id = $1 AND remind_at = $2 AND assignee_id = $3 AND NOT is_done AND (archived_at IS NULL AND activity_thread_available(thread_key))
			   FOR SHARE
			ON CONFLICT (activity_id) DO UPDATE
			   SET delivered_for = excluded.delivered_for, delivered_to = excluded.delivered_to
			 WHERE (activity_reminder_delivery.delivered_for, activity_reminder_delivery.delivered_to)
			       IS DISTINCT FROM (excluded.delivered_for, excluded.delivered_to)`,
			id, remindAt, ReminderReader(ctx))
		if err != nil {
			return fmt.Errorf("activities: claiming a reminder's notification: %w", err)
		}
		claimed = tag.RowsAffected() == 1
		return nil
	})
	return claimed, err
}
