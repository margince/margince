// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// River wiring for the automation module's clock pass: a dispatcher that
// enumerates the fleet and a workspace worker that runs one tenant's scan.
// The adapters are the only code that knows about River; the module's
// TimeScanner stays the River-agnostic seam.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TimeScanArgs schedules one clock-trigger scan pass (Task 14a): the
// coarse ActivityScan pre-filter converging every CLOCK-triggered
// automation instance (no_activity_reminder today) onto runOne — the
// same dispatch path event triggers use.
type TimeScanArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (TimeScanArgs) Kind() string { return "time_scan" }

// FleetWide marks this a dispatcher: it enumerates and enqueues,
// and does no tenant work of its own (jobs.FleetWide).
func (TimeScanArgs) FleetWide() {}

// timeScanWorker scans every live workspace.
//
// One worker where there were two (ADR-0103).
type timeScanWorker struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func (w *timeScanWorker) Work(ctx context.Context, _ *river.Job[TimeScanArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.scanWorkspace))
}

func (w *timeScanWorker) scanWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	// Built per WORKSPACE: this is a fleet pass, so the scanner's engine binds
	// the workspace this turn is for rather than whatever an installation
	// resolver would answer (ADR-0091 §9 step 3).
	db := database.BindTo(w.pool, ids.From[ids.WorkspaceKind](workspace))
	scanner := NewTimeScanner(db, w.log)
	if err := scanner.ScanWorkspace(wsCtx, workspace); err != nil {
		return err
	}
	// The lead first-response SLA is clock-triggered too (formulas §18.2)
	// and rides this pass rather than a job of its own.
	return scanLeadSLA(wsCtx, db, time.Now, w.log)
}
