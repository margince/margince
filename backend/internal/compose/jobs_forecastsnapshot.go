// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The daily forecast snapshot: the producer three shipped surfaces were waiting
// for.
//
// forecast_snapshot was written only by tests. Snapshot shares point at a frozen
// state, the movement waterfall differences two of them, and the shared CSV
// export serves one — so all three were reachable in the product and had nothing
// to serve. Every gate passed, because the suites that exercise those paths take
// their own snapshot first through the real writer.
//
// WHAT IT FREEZES, and what it does not. One snapshot per workspace per day, at
// WORKSPACE scope — the scope every one of those surfaces reads by default
// (forecasting/handlers.go resolves an absent scope_kind to it). Team and owner
// scopes are not snapshotted: nothing defaults to them, a snapshot per team and
// per owner is a deals read each, and `Movement` differences whatever two
// snapshot ids it is handed rather than demanding a scope. Adding them is a
// cadence-and-cost decision on top of this one, not part of it.
//
// The readings are assembled the way the HTTP read assembles them — resolve the
// period, read the deals in it, Compute — through the SAME two seams the handler
// takes (ForecastPeriodAt, ForecastDeals). A pass that computed its own would be
// a second answer to what the forecast is, and the frozen state would stop
// matching the live one it is supposed to be yesterday's version of.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ForecastSnapshotSweepArgs schedules one daily freeze across the fleet.
type ForecastSnapshotSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ForecastSnapshotSweepArgs) Kind() string { return "forecast_snapshot_sweep" }

// FleetWide marks this a dispatcher: it enumerates and enqueues,
// and does no tenant work of its own (jobs.FleetWide).
func (ForecastSnapshotSweepArgs) FleetWide() {}

// forecastSnapshotSweepWorker is the dispatcher: it enumerates and enqueues, and
// touches no tenant data itself.
type forecastSnapshotSweepWorker struct {
	pool *pgxpool.Pool
	now  func() time.Time
	log  *slog.Logger
}

func (w *forecastSnapshotSweepWorker) Work(ctx context.Context, _ *river.Job[ForecastSnapshotSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.snapshotWorkspace))
}

// forecastSnapshotActor is the principal the daily freeze runs as, and therefore
// the one every snapshot it writes is attributed to. Declared rather than typed
// at the call site because the suite asserting that attribution has to name the
// same principal the worker binds, and two hand-typed copies of it would agree
// only until one of them moved.
const forecastSnapshotActor = "system:forecast-snapshot"

func (w *forecastSnapshotSweepWorker) snapshotWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	wsCtx = principal.WithActor(wsCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: forecastSnapshotActor,
	})
	wsCtx = principal.WithCorrelationID(wsCtx, ids.NewV7())
	return jobs.FaultContext(ctx, w.freeze(wsCtx, workspace))
}

// freeze takes the workspace's daily snapshot for the current quarter.
//
// The QUARTER rather than the month: it is the window the forecast surfaces
// open on, and a snapshot is only worth taking of the period somebody reads.
// A month freeze would be a second cadence with its own arbiter, and nothing
// asks for one yet.
//
// One transaction, for the reason the HTTP read gives for its own: the period
// resolution, the deal read and the freeze have to see one settings state, or a
// change mid-pass labels one period's total with another period's frame.
func (w *forecastSnapshotSweepWorker) freeze(ctx context.Context, ws ids.UUID) error {
	store := forecasting.NewStore(InstallationDB(w.pool))
	at := w.now()
	var frozenDay time.Time
	err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		period, baseCurrency, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, at)
		if err != nil {
			return fmt.Errorf("resolving the period: %w", err)
		}
		scope := forecasting.Scope{Kind: forecasting.ScopeWorkspace}
		// The system principal reads every row, so `limited` cannot be true
		// here — it is the boolean a scoped HUMAN read answers with. Discarded
		// rather than stored: a snapshot taken by the fleet is of the whole
		// workspace by construction, and recording "nothing was withheld" would
		// be recording a fact about a reader this pass does not have.
		// `resolved` is the scope the read MEASURED, which is what a snapshot
		// records. A fleet pass names the workspace and the system principal
		// narrows nothing, so it answers the workspace here — but storing the
		// requested scope on that reasoning would file a measurement under a
		// population nobody had checked it against, and the seam is the only
		// thing that knows which it was.
		deals, resolved, _, err := ForecastDeals(ctx, tx, period, scope, at, baseCurrency)
		if err != nil {
			return fmt.Errorf("reading the period's deals: %w", err)
		}
		// The as-of DAY, not the instant — the same conversion the read makes,
		// and the reason it makes it: the slipped rule compares calendar days.
		readings, err := forecasting.Compute(period, period.LocalDay(at), deals)
		if err != nil {
			return fmt.Errorf("computing the readings: %w", err)
		}
		frozenDay = period.LocalDay(at)
		if _, err := store.TakeSnapshot(ctx, tx, forecasting.NewSnapshot{
			Period:       period,
			Scope:        resolved,
			Trigger:      forecasting.TriggerDaily,
			BaseCurrency: baseCurrency,
			Readings:     readings,
			TakenAt:      at,
		}); err != nil {
			return fmt.Errorf("freezing the readings: %w", err)
		}
		return nil
	})
	if alreadyFrozenToday(err) {
		// The arbiter did its job, and the whole transaction rolled back with
		// it — which is right: a day that is already frozen wants no second
		// audit row and no second set of contributions either.
		//
		// Recognised HERE rather than at the write. A unique violation aborts
		// the transaction in Postgres, so a callback that caught it and carried
		// on would reach a commit that can only roll back, and report that
		// instead — an error about the transaction, naming nothing about the
		// arbiter.
		//
		// The dispatcher ticks more than once a day so a worker that was down
		// still backfills, and River retries a failed attempt, so a second run
		// inside one local day is ordinary. Logged rather than swallowed: a
		// pass that had stopped writing entirely would otherwise look exactly
		// like one arbitrated away.
		w.log.DebugContext(ctx, "the workspace already has today's forecast snapshot",
			"workspace_id", ws, "local_day", frozenDay)
		return nil
	}
	return err
}

// alreadyFrozenToday answers whether the daily arbiter refused this write.
//
// Two indexes enforce one rule, and both have to be recognised: a partial index
// cannot cover a null scope_id, so the workspace scope — the one this pass takes
// — has an arbiter of its own beside the general one.
func alreadyFrozenToday(err error) bool {
	if err == nil {
		return false
	}
	constraint, unique := storekit.UniqueViolation(err)
	return unique &&
		(constraint == "uq_forecast_snapshot_daily" ||
			constraint == "uq_forecast_snapshot_daily_workspace")
}
