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

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DataTaskRunArgs carries no record content; durable progress belongs to the ledger.
type DataTaskRunArgs struct{}

// Kind preserves the queue identity across worker restarts.
func (DataTaskRunArgs) Kind() string { return "data_task_run" }

// FleetWide keeps the installation correction outside a user workspace queue.
func (DataTaskRunArgs) FleetWide() {}

// InsertOpts prevents parallel reconcilers from creating competing executors.
func (DataTaskRunArgs) InsertOpts() river.InsertOpts { return *dataTaskInsertOpts() }

// DataTaskReconcileArgs restores execution from the release catalog and durable ledger.
type DataTaskReconcileArgs struct{}

// Kind preserves the reconciliation queue identity.
func (DataTaskReconcileArgs) Kind() string { return "data_task_reconcile" }

// FleetWide seeds the installation ledger independently of individual seats.
func (DataTaskReconcileArgs) FleetWide() {}

// dataTaskRunLock is held by a run for as long as it executes. A run that
// cannot take it stands behind the live one, and the reconciler that can take
// it knows no run is alive.
const dataTaskRunLock = "data_task_run"

// dataTaskBusyWait is how long a run waits behind a live one.
const dataTaskBusyWait = 30 * time.Second

func dataTaskInsertOpts() *river.InsertOpts {
	return jobs.QueuedAs[DataTaskRunArgs](&river.InsertOpts{
		MaxAttempts: rowOwnedMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	})
}

func addDataTaskJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	pace := cfg.DataTasksPace
	if pace <= 0 {
		pace = time.Second
	}
	addDeclaredWorker[DataTaskRunArgs](reg, &dataTaskRunWorker{pool: pool, pace: pace, log: log})
	addDeclaredWorker[DataTaskReconcileArgs](reg, &dataTaskReconcileWorker{pool: pool, log: log})
	return periodicFor(cfg, DataTaskReconcileArgs{})
}

type dataTaskRunWorker struct {
	pool *pgxpool.Pool
	pace time.Duration
	log  *slog.Logger
}

func (w *dataTaskRunWorker) Work(ctx context.Context, _ *river.Job[DataTaskRunArgs]) error {
	release, held, err := holdSessionLock(ctx, w.pool, dataTaskRunLock, w.logger())
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if !held {
		return river.JobSnooze(dataTaskBusyWait)
	}
	defer release()
	ctx, engine, err := dataTaskJobEngine(ctx, w.pool)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	outcome, err := engine.Execute(ctx)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if outcome.Wait > 0 {
		return river.JobSnooze(outcome.Wait)
	}
	if outcome.Busy {
		return river.JobSnooze(dataTaskBusyWait)
	}
	if outcome.More {
		return river.JobSnooze(w.pace)
	}
	return nil
}

type dataTaskReconcileWorker struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func (w *dataTaskReconcileWorker) Work(ctx context.Context, _ *river.Job[DataTaskReconcileArgs]) error {
	ctx, engine, err := dataTaskJobEngine(ctx, w.pool)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if err := w.discardDeadRuns(ctx); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return jobs.FaultContext(ctx, reconcileDataTasks(ctx, engine))
}

// discardDeadRuns closes a run row a killed worker left running.
//
// River counts a running row as unique, so that row would absorb every insert
// below until the rescuer reached it an hour later. Holding the run lock proves
// no run is alive, which makes any running row a dead worker's.
func (w *dataTaskReconcileWorker) discardDeadRuns(ctx context.Context) error {
	release, held, err := holdSessionLock(ctx, w.pool, dataTaskRunLock, w.logger())
	if err != nil || !held {
		return err
	}
	defer release()
	closed, err := jobs.DiscardOrphanedRuns(ctx, w.pool, DataTaskRunArgs{}.Kind(), 0)
	if closed > 0 {
		w.logger().InfoContext(ctx, "closed data task runs left running by a dead worker", "closed", closed)
	}
	return err
}

func (w *dataTaskReconcileWorker) logger() *slog.Logger {
	if w.log == nil {
		return slog.Default()
	}
	return w.log
}

func (w *dataTaskRunWorker) logger() *slog.Logger {
	if w.log == nil {
		return slog.Default()
	}
	return w.log
}

func reconcileDataTasks(ctx context.Context, engine *datatask.Engine) error {
	if err := engine.Seed(ctx); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	progress, err := engine.List(ctx)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	for _, p := range progress {
		if p.AwaitingReview() {
			continue
		}
		if p.Status == datatask.Pending || p.Status == datatask.Running {
			client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
			if err != nil {
				return jobs.FaultContext(ctx, err)
			}
			_, err = client.Insert(ctx, DataTaskRunArgs{}, dataTaskInsertOpts())
			return jobs.FaultContext(ctx, err)
		}
	}
	return nil
}

func dataTaskJobEngine(ctx context.Context, pool *pgxpool.Pool) (context.Context, *datatask.Engine, error) {
	bound, err := installationJobCtx(ctx, identity.NewService(pool))
	if err != nil {
		return ctx, nil, err
	}
	ctx = principal.SystemActing(bound, "system:datatask")
	if _, ok := principal.CorrelationID(ctx); !ok {
		ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	}
	engine, err := newDataTaskEngine(pool, buildinfo.ReleaseVersion, time.Now)
	return ctx, engine, err
}
