// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ClaimReminderDelivery answers whether the caller is the one to show a due
// reminder's browser notification. The first claim at a reminder instant takes
// it, from any browser. A moved remind_at differs from the claimed instant, so
// the reminder is armed again.
//
// The claim admits the rows the reminder_due list returns, through its clause.
// Any other id answers false, which says nothing about a task the caller cannot
// see. The record is about a notification, not the task, so it has no audit row.
func (s *Store) ClaimReminderDelivery(ctx context.Context, id ids.ActivityID, remindAt time.Time) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	due := dueReminderClause("a", sprintf("$%d", arg(ReminderReader(ctx))), s.now(), arg)
	statement := sprintf(`
		INSERT INTO activity_reminder_delivery (activity_id, delivered_for)
		SELECT a.id, a.remind_at FROM activity a WHERE a.id = $%d AND a.remind_at = $%d AND %s
		ON CONFLICT (activity_id) DO UPDATE SET delivered_for = excluded.delivered_for
		 WHERE activity_reminder_delivery.delivered_for IS DISTINCT FROM excluded.delivered_for`,
		arg(id), arg(remindAt), due)
	var claimed bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, statement, args...)
		if err != nil {
			return fmt.Errorf("activities: claiming a reminder's notification: %w", err)
		}
		claimed = tag.RowsAffected() == 1
		return nil
	})
	return claimed, err
}
