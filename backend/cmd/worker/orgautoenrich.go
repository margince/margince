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
func startOrgAutoEnrichTrigger(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, background *sync.WaitGroup, logger *slog.Logger, stdout io.Writer) {
	inserter, err := jobs.NewInserter(pool, logger)
	if err != nil {
		// The sweep still covers every organization within a day, so a
		// missing trigger degrades promptness, not coverage — said out loud
		// because a silent absence reads exactly like a healthy lane.
		logger.Error("worker: org auto-enrich trigger not started, companies wait for the daily sweep", "err", err)
		return
	}
	trigger := compose.NewOrgAutoEnrichTrigger(pool, inserter, logger)
	_, _ = fmt.Fprintln(stdout, "worker queueing enrich passes as organizations appear (cg:org-auto-enrich)")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:org-auto-enrich", trigger.HandleEvent, logger, 0) })
}
