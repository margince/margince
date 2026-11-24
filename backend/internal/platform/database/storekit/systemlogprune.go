// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SystemLogWindow is one kind of system_log entry and how long its rows stay.
type SystemLogWindow struct {
	Name    string
	Days    int
	Actions []string
}

// SystemLogWindows lists the actions whose rows age out. An action that is not
// listed is kept, and so is any new one until it is added here. The trigger on
// system_log holds the same two lists and refuses a delete outside them.
func SystemLogWindows() []SystemLogWindow {
	return []SystemLogWindow{
		{Name: "fault and skip entries of the system log", Days: 90, Actions: []string{
			"capture_sync_error", "capture_ensure_fault", "capture_deal_attribution_fault",
			"capture_project_attribution_fault", "data_task.batch_failed", "home_repair.skipped",
			"contact_employer_repair.skipped", "dedupe_recheck", "dedupe_near_match",
			"comms_identity_reconcile_failed", "ai_task.state_changed",
		}},
		{Name: "sign-in and credential entries of the system log", Days: 400, Actions: []string{
			"login", "password_reset", "password_reset_requested", "capture.credential_rotated",
		}},
	}
}

// PruneSystemLogTx removes up to limit rows of one window that are past it,
// and answers how many. The age is taken on the database clock, which is the
// clock the trigger reads. A sign-in row whose address hash still waits for
// its key stays, because the re-key pass has not reached it.
func PruneSystemLogTx(ctx context.Context, tx pgx.Tx, window SystemLogWindow, limit int) (int64, error) {
	tag, err := tx.Exec(ctx, `
		DELETE FROM system_log WHERE id IN (
			SELECT id FROM system_log
			 WHERE action = ANY($1) AND occurred_at < now() - make_interval(days => $2)
			   AND NOT (action = 'login' AND detail ? 'email_hash' AND NOT detail ? 'email_key_id')
			 ORDER BY occurred_at LIMIT $3)`,
		window.Actions, window.Days, limit)
	if err != nil {
		return 0, fmt.Errorf("storekit: removing old system log rows: %w", err)
	}
	return tag.RowsAffected(), nil
}
