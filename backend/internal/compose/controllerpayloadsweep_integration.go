// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The payload-sweep driver the integration lane needs and nothing else does.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// DriveControllerPayloadSweepForTest runs one pass of the worker that the job
// wiring registers without send wiring. Every caller fails if that registration goes.
//
// `now` is the clock the pass reads expiry against, and opts reach the comms
// store so a test can shrink the receipt page.
func DriveControllerPayloadSweepForTest(
	ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, now time.Time, opts ...comms.StoreOption,
) error {
	reg, _ := wireJobs(pool, slog.New(slog.DiscardHandler), JobRunnerConfig{ControllerVault: vault})
	worker, wired := reg.wired[ControllerPayloadSweepArgs{}.Kind()].worker.(*controllerPayloadSweepWorker)
	if !wired {
		return errors.New("a worker without send wiring does not register comms_controller_payload_sweep")
	}
	if len(opts) > 0 {
		db := InstallationDB(pool)
		worker.store = comms.NewStore(db, time.Now, activities.NewStore(db), opts...)
	}
	worker.clock = func() time.Time { return now }
	return worker.Work(ctx, &river.Job[ControllerPayloadSweepArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 1},
	})
}
