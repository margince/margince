// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

func startAIBudgetTrigger(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, background *sync.WaitGroup, logger *slog.Logger) error {
	runner, err := jobs.NewInserter(pool, logger)
	if err != nil {
		return err
	}
	background.Go(func() {
		runSubscriber(ctx, rdb, "cg:ai-budget-resume", compose.AIBudgetResumeTrigger(runner), logger, 0)
	})
	return nil
}
