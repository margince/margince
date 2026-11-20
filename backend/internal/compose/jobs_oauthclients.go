// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unusedClientSweepBatch caps one pass, as mailDraftRetentionBatch does.
const unusedClientSweepBatch = 500

// OAuthClientUnusedSweepArgs schedules one purge of self-registered clients no
// consent used (identity.Service.PurgeUnusedDynamicClients).
type OAuthClientUnusedSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (OAuthClientUnusedSweepArgs) Kind() string { return "oauth_client_unused_sweep" }

// InsertOpts carries the attempt cap the declaration publishes.
func (OAuthClientUnusedSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, UniqueOpts: river.UniqueOpts{ByState: activeSweepStates}}
}

type unusedClientSweepWorker struct {
	identity  *identity.Service
	retention time.Duration
	log       *slog.Logger
}

func addUnusedClientSweepJob(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	retention := cfg.UnusedClientRetention
	if retention <= 0 {
		retention = identity.DefaultUnusedDynamicClientRetention
	}
	addDeclaredWorker[OAuthClientUnusedSweepArgs](reg, &unusedClientSweepWorker{
		identity: identity.NewService(pool), retention: retention, log: log,
	})
	return periodicFor(cfg, OAuthClientUnusedSweepArgs{})
}

func (w *unusedClientSweepWorker) Work(ctx context.Context, _ *river.Job[OAuthClientUnusedSweepArgs]) error {
	wsCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	wsCtx = principal.SystemActing(wsCtx, "system:oauth_client_unused_sweep")
	purged, err := w.identity.PurgeUnusedDynamicClients(wsCtx, w.retention, unusedClientSweepBatch)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if purged > 0 {
		w.log.InfoContext(wsCtx, "unused self-registered OAuth clients deleted", "clients", purged)
	}
	return nil
}
