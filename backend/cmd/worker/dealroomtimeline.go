// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the Deal Room timeline consumer.
//
// It is a consumer rather than a call inside the room's own writes because the
// note is a record of what happened, not part of the act: a timeline write that
// failed must never be able to refuse a buyer's comment. Every deal_room event
// reaches the outbox through the write shape, so one subscriber covers the
// seller's page, the buyer's page and the tool surface at once.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/gradionhq/margince/backend/internal/compose"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
)

// startDealRoomTimeline subscribes the consumer that writes what happened in a
// Deal Room onto its deal's timeline: the two sides' comments, the buyer's
// decisions on the documents, and each release.
//
// Deterministic like the other projections, so it runs on every worker: a deal
// whose lane is not running reads as a deal nobody talked about.
func startDealRoomTimeline(
	ctx context.Context,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	background *sync.WaitGroup,
	logger *slog.Logger,
	stdout io.Writer,
) {
	notes := compose.NewDealRoomTimeline(pool,
		activities.NewStore(compose.InstallationDB(pool)), logger)
	_, _ = fmt.Fprintln(stdout, "worker writing Deal Room activity onto the deal timeline")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:deal-room-timeline", notes.HandleEvent, logger, 0) })
}
