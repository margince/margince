// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the commission accrual consumer.
//
// It is a consumer rather than a call inside whatever wins a deal, because
// every path that can win one — a rep on the board, an approved agent proposal,
// an import — reaches the outbox through the write shape. Asking each writer to
// remember to accrue would guarantee one of them forgets, and a partner who was
// never paid looks exactly like a partner who earned nothing.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/modules/people"
)

// startCommissionAccrual subscribes the consumer that turns a won deal into
// what its partner earned, and reverses it when a win is undone.
//
// Deterministic like the projections beside it, so it runs on every worker: a
// workspace where nobody accrues is indistinguishable from one where no partner
// ever brought a deal.
func startCommissionAccrual(
	ctx context.Context,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	background *sync.WaitGroup,
	logger *slog.Logger,
	stdout io.Writer,
) {
	accrual := compose.NewCommissionGen(pool,
		commissions.NewStore(compose.InstallationDB(pool)),
		people.NewStore(compose.InstallationDB(pool)), logger)
	_, _ = fmt.Fprintln(stdout, "worker accruing partner commission on won deals")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:commissions", accrual.HandleEvent, logger, 0) })
}
