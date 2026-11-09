// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the consumer that holds a contact's queued marketing when they stop
// it. Without it a stopped message waits for its due time, and only the fire
// path's own check keeps it from going out.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/identity"
)

// startScheduledSendStop subscribes the stop's hold on queued marketing.
// Deterministic, so it runs on every worker.
func startScheduledSendStop(
	ctx context.Context,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	background *sync.WaitGroup,
	logger *slog.Logger,
	stdout io.Writer,
) {
	stop := compose.NewScheduledSendStop(pool, identity.NewService(pool), logger)
	_, _ = fmt.Fprintln(stdout, "worker holding queued marketing when a contact stops it (cg:scheduled-send-stop)")
	background.Go(func() {
		runSubscriber(ctx, rdb, "cg:scheduled-send-stop", stop.HandleEvent, logger, 0)
	})
}
