// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
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

type DataTaskRunArgs struct{}

func (DataTaskRunArgs) Kind() string                 { return "data_task_run" }
func (DataTaskRunArgs) FleetWide()                   {}
func (DataTaskRunArgs) InsertOpts() river.InsertOpts { return *dataTaskInsertOpts() }

type DataTaskReconcileArgs struct{}

func (DataTaskReconcileArgs) Kind() string { return "data_task_reconcile" }
func (DataTaskReconcileArgs) FleetWide()   {}

func dataTaskInsertOpts() *river.InsertOpts {
	return jobs.QueuedAs[DataTaskRunArgs](&river.InsertOpts{
		MaxAttempts: rowOwnedMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	})
}

func addDataTaskJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig) []*river.PeriodicJob {
	pace := cfg.DataTasksPace
	if pace <= 0 {
		pace = time.Second
	}
	addDeclaredWorker[DataTaskRunArgs](reg, &dataTaskRunWorker{pool: pool, pace: pace})
	addDeclaredWorker[DataTaskReconcileArgs](reg, &dataTaskReconcileWorker{pool: pool})
	return periodicFor(cfg, DataTaskReconcileArgs{})
}

type dataTaskRunWorker struct {
	pool *pgxpool.Pool
	pace time.Duration
}

func (w *dataTaskRunWorker) Work(ctx context.Context, _ *river.Job[DataTaskRunArgs]) error {
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
		return river.JobSnooze(30 * time.Second)
	}
	if outcome.More {
		return river.JobSnooze(w.pace)
	}
	return nil
}

type dataTaskReconcileWorker struct{ pool *pgxpool.Pool }

func (w *dataTaskReconcileWorker) Work(ctx context.Context, _ *river.Job[DataTaskReconcileArgs]) error {
	ctx, engine, err := dataTaskJobEngine(ctx, w.pool)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return reconcileDataTasks(ctx, engine)
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
