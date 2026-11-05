// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

const (
	// LockKey serializes correction batches across worker replicas.
	LockKey          int64 = 0x646174617461736b
	transactionLimit       = 20 * time.Second
	statementLimit         = 15 * time.Second
	estimateLimit          = 5 * time.Second
	estimateCap      int64 = 100000
)

// Outcome asks River to pace the next attempt without spending a retry.
type Outcome struct {
	More bool
	Busy bool
	Wait time.Duration
}

// Execute commits one bounded batch and its progress under the same global lock.
func (e *Engine) Execute(ctx context.Context) (Outcome, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return Outcome{}, err
	}
	batchCtx, cancel := context.WithTimeout(ctx, transactionLimit)
	defer cancel()
	var picked *selection
	var outcome Outcome
	err := database.WithWorkspaceTx(batchCtx, e.pool, func(tx pgx.Tx) error {
		var err error
		outcome, picked, err = e.executeTx(batchCtx, tx)
		return err
	})
	if err == nil || ctx.Err() != nil {
		return outcome, nil
	}
	if picked == nil {
		return Outcome{}, err
	}
	if err := e.recordFailure(ctx, *picked, err); err != nil {
		return Outcome{}, err
	}
	return Outcome{More: true}, nil
}

func lockExecutor(ctx context.Context, tx pgx.Tx) (bool, error) {
	if err := database.BoundStatement(ctx, tx, statementLimit); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '2s'`); err != nil {
		return false, err
	}
	var locked bool
	err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(@key)`, pgx.NamedArgs{"key": LockKey}).Scan(&locked)
	return locked, err
}

func (e *Engine) runBatch(ctx context.Context, tx pgx.Tx, picked selection) error {
	p := picked.progress
	if picked.task.Manual && !picked.task.hasCurrentReview(p) {
		return errors.New("manual data task requires a current reviewed activation")
	}
	if p.Activation != nil {
		remaining := int64(p.Activation.Limit) - p.Scanned
		if remaining <= 0 {
			return e.commitProgress(ctx, tx, p, BatchOut{Cursor: p.Cursor, Done: true})
		}
		p.BatchLimit = min(p.BatchLimit, int(remaining))
	}
	if p.TaskVersion != picked.task.Version {
		p.Cursor, p.Scanned, p.Changed, p.Batches = nil, 0, 0, 0
		p.Estimate, p.EstimatedAt = nil, nil
		p.BatchLimit = picked.step.Limit
		p.TaskVersion = picked.task.Version
	}
	if p.EstimatedAt == nil {
		estimate, capped, measured, err := e.boundedEstimate(ctx, tx, picked.task, picked.step, p)
		if err != nil {
			return err
		}
		now := e.now()
		p.EstimatedAt = &now
		if measured {
			if estimate < 0 || estimate > estimateCap {
				return errors.New("data task estimate exceeded its declared bound")
			}
			p.Estimate, p.EstimateCapped = &estimate, capped
		}
	}
	ctx = storekit.WithBatch(Acting(ctx, picked.task.ID), p.RunID)
	out, err := picked.step.Batch(ctx, tx, BatchIn{Cursor: p.Cursor, Limit: p.BatchLimit, RunID: p.RunID, Activation: p.Activation})
	if err != nil {
		return err
	}
	if err := validateBatch(p, out); err != nil {
		return err
	}
	if p.Activation != nil && p.Scanned+int64(out.Scanned) >= int64(p.Activation.Limit) {
		out.Done = true
	}
	if e.beforeCommit != nil {
		if err := e.beforeCommit(ctx, tx); err != nil {
			return err
		}
	}
	return e.commitProgress(ctx, tx, p, out)
}

func validateBatch(in Progress, out BatchOut) error {
	if out.Scanned < 0 || out.Changed < 0 || out.Changed > out.Scanned || out.Scanned > in.BatchLimit {
		return errors.New("data task batch exceeded its declared bound")
	}
	if len(out.Cursor) > 0 && !json.Valid(out.Cursor) {
		return errors.New("data task cursor must be JSON")
	}
	if !out.Done && (out.Scanned == 0 || len(out.Cursor) == 0 || bytes.Equal(in.Cursor, out.Cursor)) {
		return errors.New("data task batch made no cursor progress")
	}
	return nil
}

func (e *Engine) commitProgress(ctx context.Context, tx pgx.Tx, p Progress, out BatchOut) error {
	status := Running
	if out.Done {
		status = Done
	}
	tag, err := tx.Exec(ctx, `UPDATE data_task_run SET task_version = @revision, status = @status,
 cursor = @cursor, batch_limit = @limit, scanned = @scanned, changed = @changed, batches = @batches,
 estimate = @estimate, estimate_capped = @capped, estimated_at = @estimated,
 consecutive_failures = 0, last_error_class = NULL, next_attempt_at = NULL,
 last_progress_at = @now, updated_at = @now, version = version + 1
 WHERE task_id = @task AND step = @step AND version = @version`,
		pgx.NamedArgs{
			"revision": p.TaskVersion, paramStatus: status, "cursor": out.Cursor,
			paramLimit: p.BatchLimit, "scanned": p.Scanned + int64(out.Scanned), "changed": p.Changed + int64(out.Changed),
			"batches": p.Batches + 1, paramEstimate: p.Estimate, paramCapped: p.EstimateCapped, "estimated": p.EstimatedAt,
			paramNow: e.now(), paramTask: p.TaskID, paramStep: p.Step, paramVersion: p.Version,
		})
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("data task progress changed before commit")
	}
	return nil
}

func (e *Engine) executeTx(ctx context.Context, tx pgx.Tx) (Outcome, *selection, error) {
	var outcome Outcome
	var picked *selection
	locked, err := lockExecutor(ctx, tx)
	if err != nil || !locked {
		outcome.Busy = !locked && err == nil
		return outcome, picked, err
	}
	progress, err := readProgress(ctx, tx)
	if err != nil {
		return outcome, picked, err
	}
	candidate, blocked := nextStep(e.tasks, progress, e.now())
	if candidate != nil && blocked == "retry_wait" {
		outcome.Wait = candidate.progress.NextAttemptAt.Sub(e.now())
	}
	if candidate == nil || blocked != "" {
		return outcome, picked, nil
	}
	missing, err := missingRequirements(ctx, tx, candidate.task)
	if err != nil || len(missing) != 0 {
		return outcome, picked, err
	}
	picked = candidate
	current, err := scanProgress(tx.QueryRow(ctx,
		`SELECT `+progressColumns+` FROM data_task_run WHERE task_id = @task AND step = @step FOR UPDATE`,
		pgx.NamedArgs{paramTask: candidate.task.ID, paramStep: candidate.step.Name}))
	if err != nil {
		return outcome, picked, err
	}
	outcome.More = true
	if current.Version != candidate.progress.Version {
		return outcome, picked, nil
	}
	return outcome, picked, e.runBatch(ctx, tx, *picked)
}
