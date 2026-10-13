// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// DriveImportRunSweepForTest runs one sweep pass through the production
// worker, on a context that carries a River client over pool as a running job's
// does. It lives behind the integration tag, so no shipped binary links it.
func DriveImportRunSweepForTest(ctx context.Context, pool *pgxpool.Pool) error {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		return err
	}
	worker := &importRunSweepWorker{identity: identity.NewService(pool), pool: pool, log: slog.New(slog.DiscardHandler)}
	return worker.Work(rivertest.WorkContext(ctx, client),
		&river.Job[ImportRunSweepArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 1}})
}
