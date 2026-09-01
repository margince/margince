// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the consumer that closes an introduction the contact answered.
//
// It is a consumer rather than a step in whatever captures mail, because
// `replied` must rest on evidence and every path that captures a message —
// mailbox sync, an import, a rep logging a call — reaches the outbox through
// the write shape. Nothing else may set the status: an endpoint that could
// would make the workflow's best outcome the one claim nobody had to prove.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/introductions"
)

// startIntroAdvance subscribes the consumer that marks an introduction replied
// when the contact writes back.
//
// Deterministic like the projections beside it, so it runs on every worker: an
// installation where nothing advances has a queue of introductions that all
// look unanswered, which is indistinguishable from nobody ever replying.
func startIntroAdvance(
	ctx context.Context,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	background *sync.WaitGroup,
	logger *slog.Logger,
	stdout io.Writer,
) {
	advance := compose.NewIntroAdvance(pool,
		introductions.NewStore(compose.InstallationDB(pool), time.Now), logger)
	_, _ = fmt.Fprintln(stdout, "worker closing introductions the contact answered")
	background.Go(func() {
		runSubscriber(ctx, rdb, "cg:intro-advance", advance.HandleEvent, logger, 0)
	})
}
