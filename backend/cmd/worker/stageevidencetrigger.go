// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the deterministic stage-evidence writers.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
)

// startStageEvidenceTrigger starts the cg:stage-evidence consumer: a contract
// turning active, a buyer confirming a room version and a held meeting each
// become evidence against the deal's exit criteria as the record lands.
//
// No model lane is needed, unlike the enrich trigger: every claim this writes
// restates something a record already says, so it runs wherever the worker
// runs. Nothing is queued either — the work is one short transaction against
// rows the event already names, and a job would add a queue hop to it.
// No error to return, unlike the enrich trigger: that one builds a River
// inserter that can fail against the pool, and this one builds only stores.
func startStageEvidenceTrigger(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, background *sync.WaitGroup, logger *slog.Logger, stdout io.Writer) {
	trigger := compose.NewStageEvidenceTrigger(
		pool, compose.StageEvidenceDeals(pool), compose.StageEvidenceDomains(pool), logger)
	_, _ = fmt.Fprintln(stdout, "worker recording stage evidence as records land (cg:stage-evidence)")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:stage-evidence", trigger.HandleEvent, logger, 0) })
}
