// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the organization auto-enrich trigger.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/gradionhq/margince/backend/internal/compose"
	"github.com/gradionhq/margince/backend/internal/platform/jobs"
)

// startOrgAutoEnrichTrigger starts the cg:org-auto-enrich consumer: an
// organization appearing or changing queues the workspace's auto-enrich pass
// now, instead of leaving a freshly created company without a dossier until
// the next daily sweep. The runner is insert-only — the consumer queues the
// pass, and this worker's River side works it like any sweep-scheduled one.
// A failed inserter fails the boot, like the provider-enrich lane's: it only
// fails when a River client cannot be built against the pool, which is a
// process-wide misconfiguration, not one lane's weather.
func startOrgAutoEnrichTrigger(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, background *sync.WaitGroup, logger *slog.Logger, stdout io.Writer) error {
	inserter, err := jobs.NewInserter(pool, logger)
	if err != nil {
		return fmt.Errorf("worker: the enrich-trigger inserter: %w", err)
	}
	trigger := compose.NewOrgAutoEnrichTrigger(pool, inserter, logger)
	_, _ = fmt.Fprintln(stdout, "worker queueing enrich passes as organizations appear (cg:org-auto-enrich)")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:org-auto-enrich", trigger.HandleEvent, logger, 0) })
	return nil
}
