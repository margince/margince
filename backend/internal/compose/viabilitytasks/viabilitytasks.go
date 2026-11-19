// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package viabilitytasks is the data task that completes the input-check tasks
// filed before a close-date or next-step finding on a judged deal stopped
// filing one.
package viabilitytasks

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask completes each open task whose findings the viability decision now
// answers. It completes through the tick a user gives, so each task keeps its
// audit row; it never deletes one. The estimate counts tasks already done too,
// because whether a task is open is not the assurance module's fact.
func DataTask(tasks *activities.Store) datatask.Task {
	return datatask.Task{
		ID: "core/1795066312_viability_answered_tasks_completed", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1795066312_viability_answered_tasks_completed"), Requires: []string{"core/1794600201"},
		Steps: []datatask.Step{{
			Name: "complete_viability_answered_tasks", Limit: 50, MinLimit: 5,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimate, Batch: batch(tasks),
		}},
	}
}

func batch(tasks *activities.Store) func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error) {
	return func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		if err := auth.RequireSystem(ctx); err != nil {
			return datatask.BatchOut{}, err
		}
		after, err := datatask.UUIDCursor(in.Cursor)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		read, answered, err := assurance.ViabilityAnsweredTasksTx(ctx, tx, after, in.Limit)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		completed, err := tasks.CompleteSystemTasksTx(ctx, tx, answered)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		return datatask.AdvanceUUID(in, read, completed)
	}
}

func estimate(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := assurance.CountViabilityAnsweredTasks(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
