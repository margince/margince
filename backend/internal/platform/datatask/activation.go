// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const ReviewRequired = "review_required"

// AwaitingReview distinguishes dormant manual work from a paused release correction.
func (p Progress) AwaitingReview() bool { return p.BlockedReason == ReviewRequired }

// ReviewRequest identifies the exact bounded plan a human has inspected.
type ReviewRequest struct {
	Scope       json.RawMessage `json:"scope"`
	Digest      string          `json:"digest"`
	RuleVersion string          `json:"rule_version"`
	Limit       int             `json:"limit"`
	AsOf        time.Time       `json:"as_of"`
}

// Activation keeps reviewed authority with the run, across worker restarts.
type Activation struct {
	ReviewRequest
	Operator    string    `json:"operator"`
	ActivatedAt time.Time `json:"activated_at"`
}

// Activate rechecks a reviewed plan before replacing any previous manual run.
func (e *Engine) Activate(ctx context.Context, taskID, stepName string, version int64, review ReviewRequest) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "data_task", principal.ActionUpdate); err != nil {
		return err
	}
	task, step, exists := e.registeredStep(taskID, stepName)
	if !exists || !task.Manual {
		return apperrors.ErrNotFound
	}
	if review.RuleVersion != task.ReviewVersion || review.Limit < 1 || review.Limit > 1000 ||
		!json.Valid(review.Scope) || len(review.Scope) > 262144 || review.Digest == "" ||
		review.AsOf.After(e.now()) || review.AsOf.Before(e.now().Add(-15*time.Minute)) {
		return apperrors.ErrConflict
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return apperrors.ErrPermissionDenied
	}
	ctx, cancel := context.WithTimeout(ctx, transactionLimit)
	defer cancel()
	return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		if err := database.BoundStatement(ctx, tx, statementLimit); err != nil {
			return err
		}
		locked, err := lockExecutor(ctx, tx)
		if err != nil {
			return err
		}
		if !locked {
			return apperrors.ErrConflict
		}
		p, err := scanProgress(tx.QueryRow(ctx, `SELECT `+progressColumns+` FROM data_task_run
 WHERE task_id=@task AND step=@step FOR UPDATE`, pgx.NamedArgs{"task": taskID, "step": stepName}))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if p.Version != version || p.TaskVersion > task.Version {
			return apperrors.ErrConflict
		}
		if task.hasCurrentReview(p) && p.Status != Done && p.Status != Paused && p.Status != Failed {
			return apperrors.ErrConflict
		}
		missing, err := missingRequirements(ctx, tx, task)
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			return apperrors.ErrConflict
		}
		digest, err := task.Review(ctx, tx, review)
		if err != nil {
			return err
		}
		if digest != review.Digest {
			return apperrors.ErrConflict
		}
		activation := Activation{ReviewRequest: review, Operator: actor.ID, ActivatedAt: e.now()}
		return e.replaceManualRun(ctx, tx, task, step, p, activation)
	})
}

func (e *Engine) replaceManualRun(ctx context.Context, tx pgx.Tx, task Task, step Step, p Progress, activation Activation) error {
	if p.Activation != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO data_task_run_history(run_id,task_id,step,progress,preserved_at)
 SELECT run_id,task_id,step,to_jsonb(data_task_run),@now FROM data_task_run
 WHERE task_id=@task AND step=@step`, pgx.NamedArgs{"now": e.now(), "task": task.ID, "step": step.Name}); err != nil {
			return err
		}
	}
	runID := ids.NewV7()
	_, err := tx.Exec(ctx, `UPDATE data_task_run SET run_id=@run,task_version=@revision,status='pending',
 cursor=NULL,batch_limit=@limit,scanned=0,changed=0,batches=0,estimate=NULL,estimate_capped=false,
 estimated_at=NULL,consecutive_failures=0,last_error_class=NULL,next_attempt_at=NULL,last_progress_at=NULL,
 activation=@activation,updated_at=@now,version=version+1 WHERE task_id=@task AND step=@step`,
		pgx.NamedArgs{"run": runID, "revision": task.Version, "limit": step.Limit, "activation": activation, "now": e.now(), "task": task.ID, "step": step.Name})
	if err != nil {
		return err
	}
	p.RunID = runID
	return logControl(ctx, tx, p, "activate")
}

func (task Task) hasCurrentReview(p Progress) bool {
	return p.Activation != nil && p.TaskVersion == task.Version && p.Activation.RuleVersion == task.ReviewVersion
}
