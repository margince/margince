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
	"github.com/margince/margince/backend/internal/modules/identity"
)

// startAssuranceFreshness subscribes the marker that keeps a changed deal from
// reading as checked. Deterministic, so it runs on every worker: without it the
// input check's findings read as current until the next night, whatever moved.
// Coalesced, so a burst of edits to one deal is one write per read.
func startAssuranceFreshness(
	ctx context.Context,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	background *sync.WaitGroup,
	logger *slog.Logger,
	stdout io.Writer,
) {
	marker := compose.NewAssuranceFreshness(pool, identity.NewService(pool))
	_, _ = fmt.Fprintln(stdout, "worker marking input checks out of date as deals change (cg:assurance-freshness)")
	background.Go(func() {
		runCoalescingSubscriber(ctx, rdb, "cg:assurance-freshness", marker.HandleEvent, marker.HandleBatch, logger)
	})
}
