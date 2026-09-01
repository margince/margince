// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the signature-enrich trigger.

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

// startCaptureEnrichTrigger starts the cg:capture-enrich consumer: mail landing
// queues the workspace's signature pass now, instead of leaving a contact who
// wrote this morning to be read tonight.
//
// Started only where the enrich lane exists, which the caller decides. River
// DISCARDS a job whose kind no worker claims rather than holding it, and a
// discarded row is outside the uniqueness states that collapse a burst — so an
// installation with no model lane would turn every inbound mail into its own
// discarded job rather than into a queue that waits for a worker.
//
// The runner is insert-only: the consumer queues the pass, and this worker's
// River side works it like any sweep-scheduled one. A failed inserter fails
// the boot, like the organization trigger's — it only fails when a River
// client cannot be built against the pool, which is a process-wide
// misconfiguration rather than one lane's weather.
func startCaptureEnrichTrigger(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, background *sync.WaitGroup, logger *slog.Logger, stdout io.Writer) error {
	inserter, err := jobs.NewInserter(pool, logger)
	if err != nil {
		return fmt.Errorf("worker: the capture-enrich trigger inserter: %w", err)
	}
	trigger := compose.NewCaptureEnrichTrigger(pool, inserter, logger)
	_, _ = fmt.Fprintln(stdout, "worker queueing signature passes as mail lands (cg:capture-enrich)")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:capture-enrich", trigger.HandleEvent, logger, 0) })
	return nil
}
