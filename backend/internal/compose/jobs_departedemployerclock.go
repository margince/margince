// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// departedEmployerClockActor is the principal the refold runs as.
const departedEmployerClockActor = "system:departed-employer-clock"

// DepartedEmployerClockRefoldArgs refolds the company clocks a departure that
// took effect today, or in the days a missed tick skipped, left stale.
type DepartedEmployerClockRefoldArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (DepartedEmployerClockRefoldArgs) Kind() string { return "departed_employer_clock_refold" }

// InsertOpts carries the attempt cap the declaration publishes; the retry is
// the next tick, which still finds every clock this one did not reach.
func (DepartedEmployerClockRefoldArgs) InsertOpts() river.InsertOpts {
	return *jobs.QueuedAs[DepartedEmployerClockRefoldArgs](&river.InsertOpts{
		MaxAttempts: 1,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	})
}

type departedEmployerClockWorker struct {
	pool     *pgxpool.Pool
	identity *identity.Service
	log      *slog.Logger
}

func (w *departedEmployerClockWorker) Work(ctx context.Context, _ *river.Job[DepartedEmployerClockRefoldArgs]) error {
	passCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	passCtx = principal.SystemActing(passCtx, departedEmployerClockActor)
	moved, err := contacts.NewStore(InstallationDB(w.pool)).RefoldDepartedEmployerClocks(passCtx)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	w.log.InfoContext(ctx, "departed employer clock refold: company clocks moved", "count", moved)
	return nil
}
