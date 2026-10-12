// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// DriveImportRunSweepForTest runs one sweep pass through the production
// worker on the bare context River hands it. It lives
// behind the integration tag, so no shipped binary links it.
func DriveImportRunSweepForTest(ctx context.Context, pool *pgxpool.Pool) error {
	worker := &importRunSweepWorker{identity: identity.NewService(pool), pool: pool, log: slog.New(slog.DiscardHandler)}
	return worker.Work(ctx, &river.Job[ImportRunSweepArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 1}})
}
