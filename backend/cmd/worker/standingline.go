// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// startStandingLineTrigger starts the cg:standing-line consumer, which queues
// a How it stands check for each activity that lands. Only with a summarize
// lane: the check's worker registers nothing without one, and River discards
// a job no worker claims.
func startStandingLineTrigger(
	ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, readable bool,
	background *sync.WaitGroup, logger *slog.Logger, stdout io.Writer,
) error {
	if !readable {
		return nil
	}
	inserter, err := jobs.NewInserter(pool, logger)
	if err != nil {
		return fmt.Errorf("worker: the How it stands inserter: %w", err)
	}
	trigger := compose.NewStandingLineTrigger(pool, inserter, logger)
	_, _ = fmt.Fprintln(stdout, "worker checking How it stands as activities land (cg:standing-line)")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:standing-line", trigger.HandleEvent, logger, 0) })
	return nil
}
