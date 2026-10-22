// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ClaimDueJobs atomically claims up to limit due jobs. FOR UPDATE SKIP LOCKED
// keeps parallel workers from double-claiming without serializing on each
// other.
func (s *Store) ClaimDueJobs(ctx context.Context, limit int) ([]QueuedJob, error) {
	var jobs []QueuedJob
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The pick is MATERIALIZED so it runs once. An IN over a LIMITed subquery can be
		// re-run per row, and one statement then claims more rows than its limit.
		rows, err := tx.Query(ctx, `
			WITH picked AS MATERIALIZED (
			  SELECT id FROM runner_job
			  WHERE status = 'queued' AND due_at <= now()
			  ORDER BY due_at
			  LIMIT $1
			  FOR UPDATE SKIP LOCKED)
			UPDATE runner_job SET status = 'running', attempts = attempts + 1
			WHERE id IN (SELECT id FROM picked)
			RETURNING id, agent_spec, trigger_ref, passport_id`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var j QueuedJob
			if err := rows.Scan(&j.ID, &j.SpecName, &j.TriggerRef, &j.PassportID); err != nil {
				return err
			}
			jobs = append(jobs, j)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("runner: claim jobs: %w", err)
	}
	return jobs, nil
}
