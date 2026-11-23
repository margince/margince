// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// runOutcome names one terminal write onto a claimed run row.
// Each reaches the row by its (handler, idempotency_key) claim, so it needs both
// columns of workflow_run_unique. With only handler, a firing reads that
// handler's whole history. The plan-shape test (runplanshape_integration_test.go)
// explains every entry, so a statement added here is covered without being listed again.
type runOutcome string

const (
	outcomeStaged            runOutcome = "staged"
	outcomeStagedWithApplied runOutcome = "staged_with_applied"
	outcomeSkipped           runOutcome = "skipped"
	outcomeFailed            runOutcome = "failed"
	outcomeApplied           runOutcome = "applied"
)

const runClaimedWhere = `
	WHERE handler = $1 AND idempotency_key = $2`

var runOutcomeSQL = map[runOutcome]string{
	outcomeStaged:            `UPDATE workflow_run SET status = 'requires_approval', detail = $3` + runClaimedWhere,
	outcomeStagedWithApplied: `UPDATE workflow_run SET status = 'requires_approval', detail = $3, applied = $4` + runClaimedWhere,
	outcomeSkipped:           `UPDATE workflow_run SET status = 'skipped', detail = $3` + runClaimedWhere,
	outcomeFailed:            `UPDATE workflow_run SET status = 'failed', detail = $3` + runClaimedWhere,
	outcomeApplied:           `UPDATE workflow_run SET applied = $3` + runClaimedWhere,
}

// claimRunSQL is the claim insert. With reclaim, the conflict arm takes over a
// run whose Apply failed retryable inside applyRetryWindow, and the statement
// takes that window as $7.
func claimRunSQL(reclaim bool) string {
	onConflict := "DO NOTHING"
	if reclaim {
		onConflict = `DO UPDATE
			SET status = EXCLUDED.status, planned = EXCLUDED.planned, detail = NULL, applied = NULL
			WHERE workflow_run.status = 'failed' AND workflow_run.applied IS NULL
			  AND workflow_run.detail @> '{"retryable":true}'
			  AND workflow_run.created_at > now() - $7::interval`
	}
	return `
			INSERT INTO workflow_run (handler, idempotency_key, trigger_event, planned, status, detail)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (handler, idempotency_key) ` + onConflict
}
