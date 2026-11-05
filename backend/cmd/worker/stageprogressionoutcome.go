// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the consumer that counts how each proposed stage move was received.
//
// A consumer rather than a step on the decide path, because one verdict has no
// decider: the window closing on a card nobody answered is written by the
// expiry sweep and reaches the same approval.decided event. Counting only the
// answered cards would leave every ignored transition standing at `proposed`,
// which reads as a clean record rather than an unmeasured one — and the rate
// this ledger holds is what decides whether the feature may stay on.

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

// startStageProgressionOutcome subscribes the ledger's closing half.
//
// Deterministic like the projections beside it, so it runs on every worker: an
// installation where nothing closes these rows reports a stage-automation
// record of pure proposals, which is the shape of a feature nobody has ever
// agreed with and of one nobody has ever measured.
func startStageProgressionOutcome(
	ctx context.Context,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	background *sync.WaitGroup,
	logger *slog.Logger,
	stdout io.Writer,
) {
	outcome := compose.NewStageProgressionOutcome(
		pool, compose.StageEvidenceDeals(pool), identity.NewService(pool), logger)
	_, _ = fmt.Fprintln(stdout,
		"worker counting how stage moves were received (cg:stage-progression-outcome)")
	background.Go(func() {
		runSubscriber(ctx, rdb, "cg:stage-progression-outcome", outcome.HandleEvent, logger, 0)
	})
}
