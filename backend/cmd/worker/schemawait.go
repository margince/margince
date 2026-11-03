// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
)

// schemaPoll is how often a waiting worker asks whether the api has migrated.
const schemaPoll = 2 * time.Second

// awaitSchema holds the boot until the database carries every migration this
// binary ships, and reports whether it does. False means ctx ended first.
//
// The api migrates while a rolling deploy starts this role. A job reading a
// column its migration adds fails until that lands, and three failures discard
// it. There is no deadline: unready is the state compose.SchemaAtHead asks for,
// and /healthz stays live, so the orchestrator neither restarts nor routes here.
func awaitSchema(ctx context.Context, atHead func(context.Context) error, every time.Duration, log *slog.Logger) bool {
	behind := atHead(ctx)
	if behind == nil {
		return true
	}
	log.Warn("worker: waiting for the api to apply this binary's migrations before starting its lanes and jobs",
		"reason", behind.Error())
	tick := time.NewTicker(every)
	defer tick.Stop()
	for polls := 1; ; polls++ {
		select {
		case <-ctx.Done():
			return false
		case <-tick.C:
		}
		if atHead(ctx) == nil {
			log.Info("worker: the schema is at head; starting its lanes and jobs", "waited", time.Duration(polls)*every)
			return true
		}
	}
}

// observeUntilAtHead serves the probes, then waits for the schema. The probes
// come first so a waiting replica answers unready instead of nothing. The stop
// it returns is never nil, so run defers it before checking the error.
func observeUntilAtHead(ctx context.Context, cfg workerConfig, pool *pgxpool.Pool, rdb *redis.Client,
	boot *bootGate, log *slog.Logger,
) (stop func(), atHead bool, err error) {
	observe, err := startObserveListener(ctx, cfg, pool, rdb, boot, log)
	if err != nil {
		return func() {}, false, err
	}
	return observe.Stop, awaitSchema(ctx, compose.SchemaAtHead(pool), schemaPoll, log), nil
}
