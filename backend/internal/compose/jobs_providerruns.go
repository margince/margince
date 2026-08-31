// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// River wiring for provider-run execution (ADR-0101): the submit job a queued
// run commits with itself, plus a dispatcher over every LIVE workspace and a
// per-workspace drain that polls in-progress runs, recovers pending claim
// hand-offs, re-dispatches stranded submissions and expires dead in-flight
// markers. Mirrors jobs_webhookretry.go's shape: the group registers itself —
// workers and periodic schedule together.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ProviderRunsConfig is the provider-run lanes' slice of the runner's boot
// configuration. Both fields present wires the three kinds; either absent
// registers nothing — a role with no adapter compiled in, or no vault to
// unseal a credential with, has no run it could ever execute (PI-AC-9).
type ProviderRunsConfig struct {
	// Registry is the closed set of adapters this build carries.
	Registry *integrations.Registry
	// Vault custodies the provider API keys the execution paths unseal.
	Vault keyvault.Vault
}

// addProviderRunJobs registers the three workers and returns the poll
// dispatcher's schedule for the caller to append.
func addProviderRunJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig) []*river.PeriodicJob {
	if cfg.ProviderRuns.Registry == nil || cfg.ProviderRuns.Vault == nil {
		return nil
	}
	addDeclaredWorker[ProviderRunSubmitArgs](reg, &providerRunSubmitWorker{pool: pool, cfg: cfg.ProviderRuns})
	addDeclaredWorker[ProviderRunPollSweepArgs](reg, &providerRunPollSweepWorker{pool: pool})
	addDeclaredWorker[ProviderRunPollArgs](reg, &providerRunPollWorker{pool: pool, cfg: cfg.ProviderRuns})
	addDeclaredWorker[ProviderLookupSweepArgs](reg, &providerLookupSweepWorker{pool: pool})
	addDeclaredWorker[ProviderLookupArgs](reg, &providerLookupWorker{pool: pool, cfg: cfg.ProviderRuns})
	return append(periodicFor(cfg, ProviderRunPollSweepArgs{}),
		periodicFor(cfg, ProviderLookupSweepArgs{})...)
}

// providerRunStore builds the store bound to ONE workspace's DB, the way
// every fleet pass must (ADR-0091 §9 step 3). The execution paths need no
// domain callbacks of their own except at hand-off, where an unbound writer
// parks the claims for the sweep; providerDomainStore (provider.go) is the
// one place the domain edges attach for every role.
func providerRunStore(pool *pgxpool.Pool, cfg ProviderRunsConfig, args jobs.WorkspaceScoped) (*integrations.Store, error) {
	db, err := workspaceJobDB(pool, args)
	if err != nil {
		return nil, err
	}
	store, err := integrations.NewStore(db, cfg.Vault, cfg.Registry, time.Now)
	if err != nil {
		return nil, err
	}
	return bindProviderDomain(store), nil
}

// ProviderRunSubmitArgs submits ONE queued run. The workspace travels in the
// args because a job queue is not a request and carries no tenant to inherit.
type ProviderRunSubmitArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
	RunID     string   `json:"run_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (ProviderRunSubmitArgs) Kind() string { return "provider_run_submit" }

// WorkspaceID binds this submission to its tenant (jobs.WorkspaceScoped).
func (a ProviderRunSubmitArgs) WorkspaceID() ids.UUID { return a.Workspace }

// InsertOpts binds the uniqueness to the ARGS TYPE rather than to the one
// call site that enqueues today, because it is a spend guard: two live submit
// jobs for one run could each pass the queued check and buy the same answer
// twice. The state window excludes completed (activeSweepStates) so the
// stranded-submit recovery can re-enqueue after a worked job died.
func (ProviderRunSubmitArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeSweepStates}}
}

// providerRunSubmitWorker executes one submission.
type providerRunSubmitWorker struct {
	pool *pgxpool.Pool
	cfg  ProviderRunsConfig
}

func (w *providerRunSubmitWorker) Work(ctx context.Context, job *river.Job[ProviderRunSubmitArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	wsCtx = providerJobActor(wsCtx)
	store, err := providerRunStore(w.pool, w.cfg, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return jobs.FaultContext(ctx, store.ExecuteSubmit(wsCtx, job.Args.RunID))
}

// ProviderRunPollSweepArgs schedules one fleet-wide pass over due runs.
type ProviderRunPollSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ProviderRunPollSweepArgs) Kind() string { return "provider_run_poll_sweep" }

// FleetWide marks this a dispatcher: it enumerates and enqueues, and does no
// tenant work of its own (jobs.FleetWide).
func (ProviderRunPollSweepArgs) FleetWide() {}

// providerJobActor binds the principal a provider run STARTS as, plus the
// correlation id the outbox envelope requires.
//
// It names the worker, not a vendor, and that is the whole of what this layer
// knows: the submit worker holds a run id and the poll sweep drains many runs
// at once, so a connector named here could only ever be a guess. It used to be
// one — `connector:surfe`, correct only while a CHECK constraint made a second
// provider impossible. The store narrows this to the run's own connector the
// moment it has read which one that is, and every audit row a run writes is
// written after that.
//
// This binding is still load-bearing, for the reason it was added: without an
// actor the hand-off refused with "no actor bound to context" and every polled
// run stayed in_progress forever — paid for, answered, and reaching nobody. It
// is the floor under a path that reaches a gated write before it has resolved a
// provider, and it is deliberately a name no reader can mistake for a vendor.
func providerJobActor(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "provider_run_worker",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// providerRunPollSweepWorker fans one drain job out per live workspace.
type providerRunPollSweepWorker struct {
	pool *pgxpool.Pool
}

func (w *providerRunPollSweepWorker) Work(ctx context.Context, _ *river.Job[ProviderRunPollSweepArgs]) error {
	return jobs.FaultContext(ctx, dispatchPerWorkspace(ctx, w.pool,
		// The tick is the real cadence: a failed workspace is re-enqueued on
		// the next pass, so River's ladder only rides out a transient blip.
		workspaceSweepOpts(ProviderRunPollArgs{}.Kind()),
		func(ws ids.UUID) river.JobArgs { return ProviderRunPollArgs{Workspace: ws} }))
}

// ProviderRunPollArgs drains one workspace's due runs.
type ProviderRunPollArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (ProviderRunPollArgs) Kind() string { return "provider_run_poll" }

// WorkspaceID binds this drain to its tenant (jobs.WorkspaceScoped).
func (a ProviderRunPollArgs) WorkspaceID() ids.UUID { return a.Workspace }

// providerRunPollWorker drains one workspace.
type providerRunPollWorker struct {
	pool *pgxpool.Pool
	cfg  ProviderRunsConfig
}

func (w *providerRunPollWorker) Work(ctx context.Context, job *river.Job[ProviderRunPollArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	wsCtx = providerJobActor(wsCtx)
	store, err := providerRunStore(w.pool, w.cfg, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return jobs.FaultContext(ctx, store.RunDueSweep(wsCtx))
}
