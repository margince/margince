// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pastActivityClockActor writes no audit row; it names who passes
// auth.RequireSystem when the refold moves a clock.
const pastActivityClockActor = "system:past-activity-clock-refold"

// pastActivityClockBatch bounds one transaction, so no pass holds record locks
// long enough to stall the activity writes queued behind them.
const pastActivityClockBatch = 200

// PastActivityClockRefoldArgs refolds the clocks of the records an activity is
// filed under once its time has come.
type PastActivityClockRefoldArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (PastActivityClockRefoldArgs) Kind() string { return "past_activity_clock_refold" }

// InsertOpts carries the attempt cap the declaration publishes; the retry is
// the next tick, which resumes from the stored mark.
func (PastActivityClockRefoldArgs) InsertOpts() river.InsertOpts {
	return *jobs.QueuedAs[PastActivityClockRefoldArgs](&river.InsertOpts{
		MaxAttempts: 1,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	})
}

type pastActivityClockWorker struct {
	pool     *pgxpool.Pool
	identity *identity.Service
	log      *slog.Logger
}

func (w *pastActivityClockWorker) Work(ctx context.Context, _ *river.Job[PastActivityClockRefoldArgs]) error {
	passCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	passCtx = principal.SystemActing(passCtx, pastActivityClockActor)
	refolded, err := RefoldPastActivityClocks(passCtx, InstallationDB(w.pool))
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	w.log.InfoContext(ctx, "past activity clock refold: activities refolded", "count", refolded)
	return nil
}

// RefoldPastActivityClocks walks the activities whose time has come since the
// stored mark, batch by batch, and answers how many it refolded. Each batch
// refolds its records and advances the mark in one transaction, so a crash
// costs at most one batch done twice. The day entries the walk cannot time go
// first, read against the same mark.
func RefoldPastActivityClocks(ctx context.Context, db *database.DB) (int, error) {
	var mark activities.ClockRefoldMark
	var asOf time.Time
	var days activities.DayRefoldTargets
	if err := db.Tx(ctx, func(tx pgx.Tx) (err error) {
		if mark, asOf, err = activities.ClockRefoldStartTx(ctx, tx); err != nil {
			return err
		}
		zone, err := identity.ZoneAppliedTx(ctx, tx)
		if err != nil {
			return err
		}
		if days, err = activities.DayActivitiesComeTx(ctx, tx, mark, asOf, zone); err != nil || days.Count == 0 {
			return err
		}
		return refoldClocksTx(ctx, tx, days.Targets)
	}); err != nil {
		return 0, err
	}
	refolded := days.Count
	for {
		var batch activities.PassedActivities
		err := db.Tx(ctx, func(tx pgx.Tx) (err error) {
			batch, err = activities.PassedActivitiesTx(ctx, tx, mark, asOf, pastActivityClockBatch)
			if err != nil {
				return err
			}
			if err := refoldClocksTx(ctx, tx, batch.Targets); err != nil {
				return err
			}
			return activities.AdvanceClockRefoldMarkTx(ctx, tx, batch.Through)
		})
		if err != nil {
			return refolded, err
		}
		refolded += batch.Count
		if batch.Count < pastActivityClockBatch {
			return refolded, nil
		}
		mark = batch.Through
	}
}

// refoldClocksTx moves every clock a write filed under targets would move. It
// locks the whole set once, in the order every last-activity writer shares.
// So two batches that reach one record queue rather than deadlock.
func refoldClocksTx(ctx context.Context, tx pgx.Tx, targets storekit.LastActivityTargets) error {
	if err := storekit.LockLastActivityTargets(ctx, tx, targets); err != nil {
		return err
	}
	if err := contacts.RefoldClocksTx(ctx, tx, targets); err != nil {
		return err
	}
	if err := deals.RefoldClocksTx(ctx, tx, targets.Deals); err != nil {
		return err
	}
	return projects.RefoldClocksTx(ctx, tx, targets.Projects)
}
