// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const failureCap = 5

func (e *Engine) recordFailure(ctx context.Context, picked selection, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	p := picked.progress
	ctx = storekit.WithBatch(principal.SystemActing(ctx, "system:datatask:"+p.TaskID), p.RunID)
	fault := jobs.FaultContext(ctx, cause)
	detail, known := jobs.VettedFailure("data_task_run", fault.Error())
	p.LastErrorClass = nil
	if known {
		p.LastErrorClass = &detail.Class
	}
	p.ConsecutiveFailures++
	p.Status = Running
	if p.ConsecutiveFailures >= failureCap {
		p.Status = Failed
	}
	if errors.Is(cause, context.DeadlineExceeded) || storekit.IsLockTimeout(cause) || storekit.IsQueryCanceled(cause) {
		p.BatchLimit = max(picked.step.MinLimit, p.BatchLimit/2)
	}
	next := e.now().Add(time.Duration(p.ConsecutiveFailures) * 30 * time.Second)
	p.NextAttemptAt = nil
	if p.Status != Failed {
		p.NextAttemptAt = &next
	}
	return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE data_task_run SET status = @status, batch_limit = @limit,
 consecutive_failures = @failures, last_error_class = @class, next_attempt_at = @next,
 updated_at = @now, version = version + 1 WHERE task_id = @task AND step = @step AND version = @version`,
			pgx.NamedArgs{
				paramStatus: p.Status, paramLimit: p.BatchLimit, "failures": p.ConsecutiveFailures,
				"class": p.LastErrorClass, "next": p.NextAttemptAt, paramNow: e.now(),
				paramTask: p.TaskID, paramStep: p.Step, paramVersion: p.Version,
			})
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("data task failure outcome lost its version")
		}
		if _, err := storekit.LogSystem(ctx, tx, "data_task.batch_failed",
			map[string]any{paramTask: p.TaskID, paramStep: p.Step, "run_id": p.RunID, "class": p.LastErrorClass}); err != nil {
			return err
		}
		if p.Status == Failed {
			return e.notifyFailed(ctx, tx, p)
		}
		return nil
	})
}
