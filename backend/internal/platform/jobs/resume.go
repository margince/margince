// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ResumeScheduledTx only wakes the existing request; terminal jobs must never be revived.
// Callers lock the carrier first and advance its deadline in this same transaction.
func (r *Runner) ResumeScheduledTx(ctx context.Context, tx pgx.Tx, kind, field string, carrier, workspace ids.UUID) (bool, error) {
	args := []any{kind, field, carrier.String(), workspace.String()}
	query := fmt.Sprintf(`SELECT id,state::text,attempt,max_attempts FROM river_job
 WHERE kind=$%d AND args->>$%d=$%d AND args->>'workspace_id'=$%d
 AND state IN ('scheduled','retryable','available','running')
 ORDER BY id DESC LIMIT 1 FOR UPDATE`, len(args)-3, len(args)-2, len(args)-1, len(args))
	var id int64
	var state string
	var attempt, limit int
	err := tx.QueryRow(ctx, query, args...).Scan(&id, &state, &attempt, &limit)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("budget recovery: the existing job is missing or terminal")
	}
	if err != nil {
		return false, err
	}
	if state == "running" {
		return false, nil
	}
	if attempt >= limit {
		return false, fmt.Errorf("budget recovery: the existing job has exhausted its attempts")
	}
	if state == "available" {
		return true, nil
	}
	if _, err := r.client.JobRetryTx(ctx, tx, id); err != nil {
		return false, err
	}
	return true, nil
}
