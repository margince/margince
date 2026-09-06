// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the consumer that records what the installation owes a new contact.
//
// A consumer rather than a step inside the creation transaction, because every
// door that mints a person reaches the outbox through the write shape — the web
// form, an import, a capture, a promotion — and a duty opened here is opened for
// all of them without each remembering to.
//
// Deterministic like the projections beside it, so it runs on every worker. An
// installation where nothing consumes this has a notice queue that reads empty,
// which is indistinguishable from owing nobody anything — and that is the one
// answer a compliance control must never give wrongly.

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
)

// startNoticeCaseOpen subscribes the consumer that records the disclosure duty
// a newly created contact incurs.
func startNoticeCaseOpen(
	ctx context.Context,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	background *sync.WaitGroup,
	logger *slog.Logger,
	stdout io.Writer,
) {
	opener := compose.NewNoticeCaseOpen(pool, time.Now, logger)
	_, _ = fmt.Fprintln(stdout, "worker recording what new contacts are owed")
	background.Go(func() {
		runSubscriber(ctx, rdb, "cg:notice-case-open", opener.HandleEvent, logger, 0)
	})
}
