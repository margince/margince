// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// List refuses a failed ledger read rather than reporting no remaining work.
func (e *Engine) List(ctx context.Context) ([]Progress, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []Progress
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		progress, err := readProgress(ctx, tx)
		if err != nil {
			return err
		}
		out, err = e.describeProgress(ctx, tx, progress)
		return err
	})
	return out, err
}

func (e *Engine) describeProgress(ctx context.Context, tx pgx.Tx, stored map[string]Progress) ([]Progress, error) {
	out := make([]Progress, 0)
	next, blocked := nextStep(e.tasks, stored, e.now())
	for _, task := range e.tasks {
		missing, err := missingRequirements(ctx, tx, task)
		if err != nil {
			return nil, err
		}
		for _, step := range task.Steps {
			p, exists := stored[progressKey(task.ID, step.Name)]
			if !exists {
				p = Progress{TaskID: task.ID, Step: step.Name, Status: Pending, TaskVersion: task.Version, BatchLimit: step.Limit}
			}
			if !task.Manual && p.Status == Done && p.TaskVersion < task.Version {
				p.Status = Pending
			}
			p.Title = task.Title
			if task.Manual && !task.hasCurrentReview(p) {
				p.BlockedReason = ReviewRequired
				out = append(out, p)
				continue
			}
			if !complete(task, p) && next != nil {
				switch {
				case task.ID != next.task.ID || step.Name != next.step.Name:
					p.BlockedReason, p.BlockedBy = "earlier_step", progressKey(next.task.ID, next.step.Name)
				case blocked != "":
					p.BlockedReason = blocked
				case len(missing) > 0:
					p.BlockedReason, p.MissingPrerequisites = "missing_prerequisite", missing
				}
			}

			out = append(out, p)
		}
	}
	return out, nil
}
