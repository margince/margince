// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/personalchannelwire"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/personalchannels"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PersonalChannelSweepArgs deletes unclaimed chat files and expires unpulled replies.
type PersonalChannelSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (PersonalChannelSweepArgs) Kind() string { return "personal_channel_sweep" }

// InsertOpts carries the declared single attempt; the next tick is the retry.
func (PersonalChannelSweepArgs) InsertOpts() river.InsertOpts {
	return *jobs.QueuedAs[PersonalChannelSweepArgs](&river.InsertOpts{MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByState: activeSweepStates}})
}

type personalChannelSweepWorker struct {
	pool     *pgxpool.Pool
	identity *identity.Service
	log      *slog.Logger
}

func (w *personalChannelSweepWorker) Work(ctx context.Context, _ *river.Job[PersonalChannelSweepArgs]) error {
	passCtx, err := installationJobCtx(ctx, w.identity)
	passCtx = principal.SystemActing(passCtx, personalchannels.SweepActor)
	if err == nil {
		err = personalchannelwire.Sweep(passCtx, InstallationDB(w.pool), time.Now(), w.log)
	}
	return jobs.FaultContext(ctx, err)
}
