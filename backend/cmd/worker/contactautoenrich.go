// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the contact auto-enrich consumer.
//
// It is a consumer rather than a call inside any writer, and the reason is the
// one linkedinmatchgen.go already argued: matching only at write time means
// every later arrival is a match nobody will ever make. contact.created reaches
// the outbox through the write shape, so one subscriber covers manual entry,
// capture, site read, merge and import at once — and covers any writer added
// later without that writer knowing it exists.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// startContactAutoEnrich subscribes the consumer that fills a contact from what
// their employer's site already published.
func startContactAutoEnrich(
	ctx context.Context,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	background *sync.WaitGroup,
	logger *slog.Logger,
	stdout io.Writer,
) {
	enricher := compose.NewContactAutoEnrich(pool, contacts.NewStore(compose.InstallationDB(pool)), approvals.NewService(compose.InstallationDB(pool)), logger)
	_, _ = fmt.Fprintln(stdout, "worker filling contacts from their employer's published pages")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:contact-auto-enrich", enricher.HandleEvent, logger, 0) })
}
