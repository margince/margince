// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leasedRecords is the ledger of a worker's commit slice. Every write it makes
// is fenced on the lease, and it writes the run row at each flush, never per row.
type leasedRecords struct {
	runs  *RunStore
	lease Lease
}

// NewLeasedEngine wires an engine whose every run write is fenced on the lease.
func NewLeasedEngine(runs *RunStore, l Lease, w Writers) *Engine {
	return &Engine{runs: runs, w: w, lease: &leasedRecords{runs: runs, lease: l}}
}

func (r *leasedRecords) open(ctx context.Context, id RunID) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return Run{}, err
	}
	if id != r.lease.Run {
		return Run{}, fmt.Errorf("import run %s is not the leased run %s: %w", id, r.lease.Run, ErrSuperseded)
	}
	var run Run
	err := r.runs.tx(ctx, func(tx pgx.Tx) error {
		var err error
		run, err = readLeased(ctx, tx, r.lease)
		return err
	})
	return run, err
}

// replayed reads the rows bound to this run after its last flush: an attempt
// landed them and died before it counted them.
func (r *leasedRecords) replayed(ctx context.Context) (map[string]bool, error) {
	landed := map[string]bool{}
	err := r.runs.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT m.object || '/' || m.external_id
			  FROM import_record_map m
			  JOIN import_run r ON r.id = m.import_run_id
			 WHERE r.id = $1 AND m.created_at > coalesce(r.progress_at, '-infinity')`, r.lease.Run)
		if err != nil {
			return fmt.Errorf("reading the rows import run %s landed since its last flush: %w", r.lease.Run, err)
		}
		keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("reading the rows import run %s landed since its last flush: %w", r.lease.Run, err)
		}
		for _, key := range keys {
			landed[key] = true
		}
		return nil
	})
	return landed, err
}

func (*leasedRecords) landed(context.Context, *walk) error { return nil }

func (r *leasedRecords) moved(ctx context.Context, wk *walk) error {
	wk.moves++
	wk.sinceFlush++
	if wk.budget.flushDue(wk.sinceFlush, wk.budget.now().Sub(wk.flushedAt)) {
		return r.flush(ctx, wk)
	}
	return nil
}

func (r *leasedRecords) stop(ctx context.Context, wk *walk) (bool, error) {
	if ctx.Err() != nil {
		return true, errors.Join(ctx.Err(), r.stopFlush(ctx, wk))
	}
	if wk.budget.spent(wk.moves) {
		return true, r.flush(ctx, wk)
	}
	return false, nil
}

func (r *leasedRecords) flush(ctx context.Context, wk *walk) error {
	if wk.cursor == wk.legStart {
		return nil
	}
	if err := r.runs.Flush(ctx, r.lease, wk.flush()); err != nil {
		return err
	}
	wk.startLeg()
	return nil
}

// stopFlush lands what a stopped slice did, on a context of its own. The rows
// already written are then counted before the job snoozes or fails.
func (r *leasedRecords) stopFlush(ctx context.Context, wk *walk) error {
	flushCtx, cancel := flushContext(ctx)
	defer cancel()
	return r.flush(flushCtx, wk)
}

func (r *leasedRecords) complete(ctx context.Context, wk *walk) error {
	return r.runs.tx(ctx, func(tx pgx.Tx) error {
		return completeLeaseInTx(ctx, tx, r.lease, wk.flush())
	})
}

// fail fails the run for a cause of its own. A stopped context is no such
// cause: the slice flushes, and the job decides between a snooze and a failure.
func (r *leasedRecords) fail(ctx context.Context, wk *walk, cause error) error {
	if ctx.Err() != nil {
		return r.stopFlush(ctx, wk)
	}
	return r.runs.tx(ctx, func(tx pgx.Tx) error {
		if err := flushInTx(ctx, tx, r.lease, wk.flush()); err != nil {
			return err
		}
		return failLeaseInTx(ctx, tx, r.lease, stopCause(cause))
	})
}
