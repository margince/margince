// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// NoEffectRunDays is how long a run that changed nothing stays. Its row stops
// a second delivery of its event from running the handler twice. A delivered
// event is gone after the same number of days.
const NoEffectRunDays = 30

// runsReadLater are the handlers whose old runs a later request still reads.
// The lead rules say who claimed a website lead, and auth.LiveIntakeTaskClause
// reads a route_lead run's planned target. The inquiry rule is read by its key.
func runsReadLater() []string {
	return append(LegacyLeadHandlerNames(), inquiryRecipeName)
}

// PruneNoEffectRunsTx removes up to limit runs from before cutoff that changed
// nothing: skipped, blocked, or applied with no effect. A failed run stays for
// its retry, and a run waiting on an approval stays until it is decided.
func PruneNoEffectRunsTx(ctx context.Context, tx pgx.Tx, cutoff time.Time, limit int) (int64, error) {
	tag, err := tx.Exec(ctx, `DELETE FROM workflow_run WHERE id IN (
		SELECT id FROM workflow_run
		 WHERE created_at < @cutoff AND handler <> ALL(@read_later::text[])
		   AND (status IN ('`+runSkipped+`', '`+runBlocked+`')
		        OR (status = '`+runStatusApplied+`' AND (applied IS NULL OR applied IN ('null'::jsonb, '[]'::jsonb))))
		 ORDER BY id LIMIT @limit)`,
		pgx.StrictNamedArgs{"cutoff": cutoff, "read_later": runsReadLater(), "limit": limit})
	if err != nil {
		return 0, fmt.Errorf("automation: removing runs that changed nothing: %w", err)
	}
	return tag.RowsAffected(), nil
}
