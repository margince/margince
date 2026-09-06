// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The payload-sweep driver the integration lane needs and nothing else does.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// DriveControllerPayloadSweepForTest runs one pass of the sweep.
//
// It builds the REAL worker rather than reimplementing its loop, and takes no
// workspace: the worker resolves the installation itself, and a helper that
// supplied one would prove the helper works while production resolved nothing.
//
// `now` is the clock the pass reads expiry against, so a test can place material
// on either side of it without sleeping.
func DriveControllerPayloadSweepForTest(
	ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, now time.Time,
) error {
	db := InstallationDB(pool)
	worker := newControllerPayloadSweepWorker(
		identity.NewService(pool),
		comms.NewStore(db, time.Now, activities.NewStore(db)),
		controllerPayloads{v: vault},
		slog.New(slog.DiscardHandler))
	worker.clock = func() time.Time { return now }
	return worker.Work(ctx, &river.Job[ControllerPayloadSweepArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 1},
	})
}
