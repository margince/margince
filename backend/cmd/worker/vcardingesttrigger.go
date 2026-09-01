// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the mailed-card import trigger.

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

// startVCardIngestTrigger starts the cg:vcard-ingest consumer: a .vcf attached
// to captured mail becomes a contact.
//
// Started UNCONDITIONALLY, unlike the signature trigger beside it. That one is
// gated on the enrich lane because River discards a job whose kind no worker
// claims — so a queued pass with no model behind it would be discarded rather
// than held. This trigger's worker is registered unconditionally for the reason
// the org-name promotion is: a card is parsed, not inferred, so there is no
// model to be missing and nothing to gate on.
func startVCardIngestTrigger(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, background *sync.WaitGroup, logger *slog.Logger, stdout io.Writer) error {
	inserter, err := jobs.NewInserter(pool, logger)
	if err != nil {
		return fmt.Errorf("worker: the vcard-ingest trigger inserter: %w", err)
	}
	trigger := compose.NewVCardIngestTrigger(pool, inserter, logger)
	_, _ = fmt.Fprintln(stdout, "worker importing mailed cards as they land (cg:vcard-ingest)")
	background.Go(func() { runSubscriber(ctx, rdb, "cg:vcard-ingest", trigger.HandleEvent, logger, 0) })
	return nil
}
