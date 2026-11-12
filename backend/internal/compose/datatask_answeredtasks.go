// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

func answeredTasksDataTask(pool *pgxpool.Pool) datatask.Task {
	store := activities.NewStore(InstallationDB(pool))
	return datatask.Task{ID: "core/1794415833_complete_answered_tasks", Version: 1, Title: systemtext.Literal("datatask.title.core.1794415833_complete_answered_tasks"), Steps: []datatask.Step{{
		Name: "complete_answered_tasks", Limit: 25, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true}, Estimate: activities.CountAnsweredTaskCandidates,
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			scanned, completed, err := store.CompleteAnsweredTasksTx(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, scanned, completed)
		},
	}}}
}
