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

// SessionLocked marks work whose execution lease, rather than a River row,
// prevents concurrent runs. A dead owner's connection releases that lease.
type SessionLocked interface{ SessionLocked() }

// SessionInsertOpts lets a released connection admit work past a stale running row.
func SessionInsertOpts(args river.JobArgs, opts *river.InsertOpts) *river.InsertOpts {
	if _, locked := args.(SessionLocked); !locked {
		return opts
	}
	owned := river.InsertOpts{}
	if opts != nil {
		owned = *opts
	}
	owned.UniqueOpts = river.UniqueOpts{}
	return &owned
}

func (r *Runner) insertSessionTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (bool, error) {
	encoded, err := json.Marshal(args)
	if err != nil {
		return false, err
	}
	bindings := pgx.StrictNamedArgs{"key": "jobs:waiting:" + args.Kind() + ":" + string(encoded)}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(@key, 0))`, bindings); err != nil {
		return false, err
	}
	var waiting bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM river_job
  WHERE kind = @kind AND args = @args::jsonb
  AND state IN ('available','scheduled','retryable','pending'))`, pgx.StrictNamedArgs{"kind": args.Kind(), "args": encoded}).Scan(&waiting)
	if err != nil || waiting {
		return false, err
	}
	result, err := r.client.InsertTx(ctx, tx, args, SessionInsertOpts(args, opts))
	if err != nil {
		return false, fmt.Errorf("jobs: enqueue %s transactionally: %w", args.Kind(), err)
	}
	return !result.UniqueSkippedAsDuplicate, nil
}
