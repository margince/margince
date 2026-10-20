// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// DataTaskOperation shares the worker's catalog with the owner-credential CLI; it never executes a batch.
func DataTaskOperation(ctx context.Context, pool *pgxpool.Pool, action string) ([]datatask.Progress, error) {
	ctx, engine, err := dataTaskJobEngine(ctx, pool)
	if err != nil {
		return nil, err
	}
	if action == "run" {
		if err := enqueueDataTaskReconcile(ctx, pool); err != nil {
			return nil, err
		}
	} else if action != "status" && action != "estimate" {
		return nil, fmt.Errorf("choose data-tasks status, estimate or run")
	}
	progress, err := engine.List(ctx)
	if err != nil {
		return nil, err
	}
	if action == "estimate" {
		for _, p := range progress {
			if p.Version == 0 {
				continue
			}
			if err := engine.Control(ctx, p.TaskID, p.Step, "estimate", p.Version); err != nil {
				return nil, err
			}
		}
		return engine.List(ctx)
	}
	return progress, nil
}

func enqueueDataTaskReconcile(ctx context.Context, pool *pgxpool.Pool) error {
	inserter, err := jobs.NewInserter(pool, slog.Default())
	if err != nil {
		return err
	}
	return inserter.Enqueue(ctx, DataTaskReconcileArgs{}, nil)
}
