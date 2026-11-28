// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Zalo Official Account edge's jobs and wiring. The webhook edge and the
// worker that reads a stored message back live in zaloedge. The token renewal
// is capture.ZaloRenewer.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/zaloedge"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ZaloOAIngestArgs names one webhook message the edge stored. The worker reads
// it back by RawCaptureID.
type ZaloOAIngestArgs struct {
	ConnectionID ids.UUID `json:"connection_id"`
	RawCaptureID ids.UUID `json:"raw_capture_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (ZaloOAIngestArgs) Kind() string { return "zalo_oa_ingest" }

// RawCapture names the stored message.
func (a ZaloOAIngestArgs) RawCapture() ids.UUID { return a.RawCaptureID }

// Connection names the connection the message was stored under.
func (a ZaloOAIngestArgs) Connection() ids.UUID { return a.ConnectionID }

// ZaloTokenRenewArgs schedules one pass that renews every due access token.
type ZaloTokenRenewArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ZaloTokenRenewArgs) Kind() string { return "zalo_token_renew" }

// FleetWide marks the pass as one that walks every workspace (jobs.FleetWide).
func (ZaloTokenRenewArgs) FleetWide() {}

type zaloTokenRenewWorker struct {
	pool    *pgxpool.Pool
	renewer *capture.ZaloRenewer
}

func (w *zaloTokenRenewWorker) Work(ctx context.Context, _ *river.Job[ZaloTokenRenewArgs]) error {
	return jobs.FaultContext(ctx, runInstallationPass(ctx, w.pool, w.renewer.RenewWorkspace))
}

// registerZaloTokenRenew wires the renewal pass. It needs the vault, because the
// pair it replaces is sealed there.
func registerZaloTokenRenew(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	if cfg.ChannelVault == nil {
		return nil
	}
	addDeclaredWorker[ZaloTokenRenewArgs](reg, &zaloTokenRenewWorker{pool: pool, renewer: &capture.ZaloRenewer{
		Pool: pool, Vault: cfg.ChannelVault, Client: zalo.NewClient(nil, "", ""), Log: log, Now: time.Now,
	}})
	return periodicFor(cfg, ZaloTokenRenewArgs{})
}

// WithZaloOAEdge composes Zalo's webhook for Official Accounts. An event stores
// its message and queues its ingest through inserter. The vault is read per
// call, so this option may come before WithKeyvault.
func WithZaloOAEdge(inserter *jobs.Runner) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		db := InstallationDB(pool)
		s.channelHandlers = s.WithZaloEvents(capture.NewZaloEventHandlers(&zaloedge.Receiver{
			Endpoints: capture.NewZaloEdge(db, func() keyvault.Vault { return s.vault }, s.log),
			DB:        db,
			Inserter:  inserter,
			Job: func(workspace, connection, rawCapture ids.UUID) river.JobArgs {
				return ZaloOAIngestArgs{ConnectionID: connection, RawCaptureID: rawCapture}
			},
			MaxAttempts: oneOffJobMaxAttempts,
			Log:         s.log,
		}))
	}
}
