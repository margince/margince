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
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// startStageEvidenceTrigger starts the cg:stage-evidence consumer: a contract
// turning active and a meeting that took place each become evidence against
// the deal's exit criteria as the record lands.
//
// It runs UNGATED. Every claim the deterministic half writes restates
// something a record already says, so it needs no model lane and gating it
// would delete the feature in an AI-less deployment for a reason that is not
// its own. That half is queued nowhere either — the work is one short
// transaction against rows the event already names.
//
// The MODEL's half is different and is why this takes a `readable` flag. It is
// a queued reading of what was actually said, and its worker registers only
// where a lane exists (jobs.yaml: registers_nothing). River DISCARDS a job
// whose kind no worker claims rather than holding it, so queueing one on a
// lane-less installation would turn every activity on a deal into a discarded
// row. Nil inserter, no reading queued, and the deterministic evidence stands
// on its own — which is exactly what such an installation should get.
func startStageEvidenceTrigger(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, readable bool, background *sync.WaitGroup, logger *slog.Logger, stdout io.Writer) error {
	var inserter compose.StageEvidenceReadEnqueuer
	if readable {
		built, err := jobs.NewInserter(pool, logger)
		if err != nil {
			return fmt.Errorf("worker: the stage-evidence reading inserter: %w", err)
		}
		inserter = built
	}
	trigger := compose.NewStageEvidenceTrigger(
		pool, compose.StageEvidenceDeals(pool), compose.StageEvidenceDomains(pool), inserter, logger)
	_, _ = fmt.Fprintln(stdout, "worker recording stage evidence as records land (cg:stage-evidence)")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:stage-evidence", trigger.HandleEvent, logger, 0) })
	return nil
}
