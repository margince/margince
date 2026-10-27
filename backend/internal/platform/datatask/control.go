// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EstimateAction is shared by operator adapters and ledger audit actions.
const EstimateAction = "estimate"

// Control requires the version the operator read, so a stale click cannot replace progress.
func (e *Engine) Control(ctx context.Context, taskID, stepName, action string, version int64) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "data_task", principal.ActionUpdate); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, transactionLimit)
	defer cancel()
	task, step, exists := e.registeredStep(taskID, stepName)
	if !exists {
		return apperrors.ErrNotFound
	}
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		if err := database.BoundStatement(ctx, tx, statementLimit); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '2s'`); err != nil {
			return err
		}
		p, err := scanProgress(tx.QueryRow(ctx,
			`SELECT `+progressColumns+` FROM data_task_run WHERE task_id = @task AND step = @step FOR UPDATE`,
			pgx.NamedArgs{paramTask: taskID, paramStep: stepName}))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if p.Version != version || p.TaskVersion > task.Version {
			return apperrors.ErrConflict
		}
		if action == EstimateAction {
			return e.refreshEstimate(ctx, tx, task, step, p)
		}
		return e.changeStatus(ctx, tx, p, action)
	})
	if storekit.IsLockTimeout(err) {
		return apperrors.ErrConflict
	}
	return err
}

func controlStatus(status, action string) (string, error) {
	switch {
	case action == "pause" && (status == Pending || status == Running):
		return Paused, nil
	case action == "resume" && status == Paused:
		return Pending, nil
	case action == "retry" && status == Failed:
		return Pending, nil
	default:
		return "", fmt.Errorf("refresh this correction before changing its state: %w", apperrors.ErrConflict)
	}
}

func (e *Engine) registeredStep(taskID, stepName string) (Task, Step, bool) {
	for _, task := range e.tasks {
		if task.ID != taskID {
			continue
		}
		for _, step := range task.Steps {
			if step.Name == stepName {
				return task, step, true
			}
		}
	}
	return Task{}, Step{}, false
}

func (e *Engine) refreshEstimate(ctx context.Context, tx pgx.Tx, task Task, step Step, p Progress) error {
	missing, err := missingRequirements(ctx, tx, task)
	if err != nil {
		return err
	}
	if len(missing) != 0 {
		return apperrors.ErrConflict
	}
	if err := database.BoundStatement(ctx, tx, statementLimit); err != nil {
		return err
	}
	estimate, capped, err := e.estimateStep(ctx, tx, task, step, p)
	if err != nil {
		return err
	}
	if estimate < 0 || estimate > estimateCap {
		return errors.New("data task estimate exceeded its declared bound")
	}
	tag, err := tx.Exec(ctx, `UPDATE data_task_run SET estimate = @estimate, estimate_capped = @capped,
 estimated_at = @now, updated_at = @now, version = version + 1
 WHERE task_id = @task AND step = @step AND version = @version`,
		pgx.NamedArgs{paramEstimate: estimate, paramCapped: capped, paramNow: e.now(), paramTask: p.TaskID, paramStep: p.Step, paramVersion: p.Version})
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperrors.ErrConflict
	}
	return logControl(ctx, tx, p, EstimateAction)
}

func logControl(ctx context.Context, tx pgx.Tx, p Progress, action string) error {
	_, err := storekit.LogSystem(ctx, tx, "data_task."+action,
		map[string]any{paramTask: p.TaskID, paramStep: p.Step, "run_id": p.RunID, paramVersion: p.Version})
	return err
}

func (e *Engine) changeStatus(ctx context.Context, tx pgx.Tx, p Progress, action string) error {
	status, err := controlStatus(p.Status, action)
	if err != nil {
		return err
	}
	failures := p.ConsecutiveFailures
	if action == "retry" {
		failures = 0
	}
	tag, err := tx.Exec(ctx, `UPDATE data_task_run SET status = @status,
 consecutive_failures = @failures, next_attempt_at = NULL,
 last_error_class = CASE WHEN @action = 'retry' THEN NULL ELSE last_error_class END,
 updated_at = @now, version = version + 1
 WHERE task_id = @task AND step = @step AND version = @version`,
		pgx.NamedArgs{"action": action, paramStatus: status, "failures": failures, paramNow: e.now(), paramTask: p.TaskID, paramStep: p.Step, paramVersion: p.Version})
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperrors.ErrConflict
	}
	return logControl(ctx, tx, p, action)
}

func (e *Engine) estimateStep(ctx context.Context, tx pgx.Tx, task Task, step Step, p Progress) (int64, bool, error) {
	if task.Manual {
		if !task.hasCurrentReview(p) {
			return 0, false, apperrors.ErrConflict
		}
		return int64(p.Activation.Limit), false, nil
	}
	return step.Estimate(ctx, tx, estimateCap)
}

// boundedEstimate is the estimate a batch reads before a step's first rows. The
// count only shows an operator how far the step has to go, so one that cannot
// finish within estimateLimit leaves the step unestimated, answering
// measured=false, instead of failing every batch of the step. It runs in a
// savepoint because a stopped statement aborts the transaction around it.
func (e *Engine) boundedEstimate(ctx context.Context, tx pgx.Tx, task Task, step Step, p Progress) (estimate int64, capped, measured bool, err error) {
	if task.Manual {
		estimate, capped, err = e.estimateStep(ctx, tx, task, step, p)
		return estimate, capped, err == nil, err
	}
	_, err = storekit.Savepoint(ctx, tx, func(nested pgx.Tx) (bool, error) {
		if err := database.BoundStatement(ctx, nested, estimateLimit); err != nil {
			return false, err
		}
		estimate, capped, err = step.Estimate(ctx, nested, estimateCap)
		return err == nil, err
	})
	// SET LOCAL outlives a released savepoint; the batch keeps its own ceiling.
	if bound := database.BoundStatement(ctx, tx, statementLimit); bound != nil {
		return 0, false, false, bound
	}
	if storekit.IsQueryCanceled(err) && ctx.Err() == nil {
		return 0, false, false, nil
	}
	return estimate, capped, err == nil, err
}
