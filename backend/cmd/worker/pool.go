// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/database"
)

func openWorkerPool(ctx context.Context, cfg workerConfig, log *slog.Logger) (*pgxpool.Pool, error) {
	pool, err := database.NewWorkerPool(ctx, cfg.dsn, compose.WorkerQueueConcurrency(), cfg.databaseConnections, cfg.databaseCache)
	if err != nil {
		return nil, err
	}
	log.Info("worker database budget", "max_pooled_connections", pool.Config().MaxConns,
		"max_connections_with_listener", pool.Config().MaxConns+database.WorkerListenerConnections,
		"queue_workers", compose.WorkerQueueConcurrency(), "session_lock_slots", database.WorkerSessionCapacity(pool.Config().MaxConns),
		"statement_cache_capacity", pool.Config().ConnConfig.StatementCacheCapacity)
	if err := assertMayWorkAgainst(ctx, pool, log); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
