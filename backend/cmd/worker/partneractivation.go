// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// startPartnerActivation subscribes the consumer that activates a partner on
// the first won deal it sourced. It runs on every worker, as the accrual
// beside it does: a lane nobody runs would leave every partner unactivated.
func startPartnerActivation(
	ctx context.Context,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	background *sync.WaitGroup,
	logger *slog.Logger,
	stdout io.Writer,
) {
	activation := compose.NewPartnerActivation(pool, contacts.NewStore(compose.InstallationDB(pool)), logger)
	_, _ = fmt.Fprintln(stdout, "worker activating partners on their first sourced win")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:partner-activation", activation.HandleEvent, logger, 0) })
}
