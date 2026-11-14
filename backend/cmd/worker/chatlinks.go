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
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// startChatLinkSync subscribes the consumer that queues what happens on a
// linked deal or project for its chat channel. Without the vault no bot token
// can be unsealed, so nothing could ever be posted and the consumer stays off.
func startChatLinkSync(
	ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, vault keyvault.Vault, inserter *jobs.Runner,
	posture runtimeenv.Environment, background *sync.WaitGroup, logger *slog.Logger, stdout io.Writer,
) {
	if vault == nil || inserter == nil {
		_, _ = fmt.Fprintln(stdout, "worker posting to chat channels: off (no credential store)")
		return
	}
	sync := compose.NewChatLinkSync(pool, vault, inserter, posture.IsNonProduction())
	_, _ = fmt.Fprintln(stdout, "worker queueing record updates for linked chat channels (cg:chat-links)")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:chat-links", sync.HandleEvent, logger, 0) })
}
