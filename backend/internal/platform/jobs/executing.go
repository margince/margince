// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// IsRunningTx distinguishes a lost execution from a durable job waiting for its due time.
func IsRunningTx(ctx context.Context, tx pgx.Tx, job river.JobArgs) (bool, error) {
	encoded, err := json.Marshal(job)
	if err != nil {
		return false, err
	}
	args := []any{job.Kind(), encoded}
	query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM river_job WHERE kind=$%d AND args @> $%d::jsonb AND state='running')`, len(args)-1, len(args))
	var running bool
	err = tx.QueryRow(ctx, query, args...).Scan(&running)
	return running, err
}
