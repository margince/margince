// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Capture's job surface, in two halves. How mail gets pulled on a schedule —
// the Gmail dispatch pass, the per-connection sync it fans out to, and the
// push-watch renewal that keeps Gmail notifying us at all — and what then
// happens to a captured message, which is the pipeline the second half wires.
// jobs.go composes every job and is the home of none.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// addGmailCaptureJobs registers every worker that reaches a Google mailbox,
// and registers none of them without the connector registry: each one resolves
// a connection and its credentials through it, so a role with no OAuth app
// configured could only fail whatever it picked up. The push-watch pair takes a
// SECOND condition — a Pub/Sub topic — because a watch registered against no
// topic notifies nobody; a deployment that never opted into push keeps the
// poll, which is the rest of this block.
//
// The dispatchers' schedules stay in the runner's own list, the posture the
// overlay and embed-reindex wiring take: periodicFor resolves them from the
// same declaration this gate reads.
func addGmailCaptureJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) {
	if cfg.GmailRegistry == nil {
		return
	}
	digests := &captureDigestWorker{registry: cfg.GmailRegistry, pool: pool, log: log}
	addDeclaredWorker[CaptureDigestArgs](reg, digests)
	addDeclaredWorker[CaptureDigestWorkspaceArgs](reg, &captureDigestWorkspaceWorker{digests: digests})
	addDeclaredWorker[GmailSyncArgs](reg, &gmailSyncWorker{registry: cfg.GmailRegistry, log: log})
	// The sync dispatcher scans every registered connector, so a Google
	// Calendar connection (the same Google OAuth app) syncs on the identical
	// per-connection path a mailbox does — there is no gcal-specific job.
	// Per-connection pacing lives in the registry's scheduling sidecar
	// (next_sync_at = success + --gmail-sync-interval), which is why the
	// dispatcher's own cadence can be frequent without meaning frequent
	// provider calls.
	addDeclaredWorker[CaptureSyncArgs](reg, &captureSyncWorker{registry: cfg.GmailRegistry, log: log})
	// Backfill jobs are enqueued by the api (start op); the worker role only
	// needs the pager registered.
	addDeclaredWorker[CaptureBackfillArgs](reg, &captureBackfillWorker{registry: cfg.GmailRegistry, log: log})
	if cfg.GmailWatch.Topic == "" {
		return
	}
	addDeclaredWorker[GmailWatchArgs](reg, &gmailWatchWorker{
		registry: cfg.GmailRegistry, renewWithin: cfg.GmailWatch.RenewWithin, log: log,
	})
	addDeclaredWorker[GmailWatchRenewArgs](reg, &gmailWatchRenewWorker{
		registry: cfg.GmailRegistry, topic: cfg.GmailWatch.Topic,
	})
}

// addGraphWatchJobs registers the Microsoft Graph subscription-renewal pair.
//
// Its own function rather than a branch inside the Gmail block: the two share a
// registry and nothing else, and the Graph pass takes a SECOND condition of its
// own — a notification URL. A subscription registered against no URL notifies
// nobody, so a deployment that never opted into Outlook push keeps the poll.
func addGraphWatchJobs(reg *jobRegistry, cfg JobRunnerConfig, log *slog.Logger) {
	if !GraphWatchWillRun(cfg.GmailRegistry, cfg.GraphWatch) {
		return
	}
	addDeclaredWorker[GraphWatchArgs](reg, &graphWatchWorker{
		registry: cfg.GmailRegistry, renewWithin: cfg.GraphWatch.RenewWithin, log: log,
	})
	addDeclaredWorker[GraphWatchRenewArgs](reg, &graphWatchRenewWorker{
		registry: cfg.GmailRegistry, notificationURL: cfg.GraphWatch.NotificationURL,
	})
}

// GraphWatchWillRun reports whether this build registers the Graph
// subscription-renewal pair.
//
// A NOTIFICATION URL IS NOT ENOUGH, and a connector to renew through is the
// other half. Both workers resolve the Graph connector out of the registry, so
// without one every renewal fails on a connector that was never registered —
// for every Outlook connection, on every scan, forever. A pass that can only
// fail is worse than no pass: the poll still runs either way, and the difference
// is a fleet-wide error rate an operator has to explain.
//
// The worker registers Graph from ENVIRONMENT credentials alone
// (CaptureSyncRegistry), where the Google side also resolves a stored app. So a
// Microsoft app configured in the product reaches the api and not this role, and
// an operator who sets the notification URL for it gets exactly that failing
// pass.
//
// EXPORTED so the boot banner can ask the same question rather than restate it.
// A banner naming a lane that did not come up is worse than no banner: it is the
// one place an operator looks to check.
//
// ASKED OF THE DECLARATION, not of the fields directly. The periodic schedule
// gates on api/jobs.yaml's registration.when through the same `registers`, so a
// condition spelled only here would be one the scheduler does not know about —
// and it enqueues under its own answer. That divergence does not fail loudly:
// the rows are inserted, no worker claims them, and the lane looks idle rather
// than broken.
func GraphWatchWillRun(reg *capture.Registry, cfg GraphWatchConfig) bool {
	spec, declared := jobs.SpecFor(graphWatchKind)
	if !declared {
		panic("compose: api/jobs.yaml does not declare " + graphWatchKind)
	}
	return registers(JobRunnerConfig{GmailRegistry: reg, GraphWatch: cfg}, spec.Registration)
}

// registryOffers reports whether reg holds a connector for provider.
//
// Asked of the registry rather than of the config that built it: what decides
// whether a job can run is the connector it will look up, and the two have
// already disagreed once — a stored app registers a connector no environment
// variable names.
func registryOffers(reg *capture.Registry, provider string) bool {
	for _, desc := range reg.Connectors() {
		if desc.Name == provider {
			return true
		}
	}
	return false
}

// addCapturePipelineJobs registers what surrounds the pull above: the Telegram
// ingest that admits a message, the passes that read one already captured, and
// the outbound send.
//
// Every gate here is on its own block, which is what tells this half from the
// Gmail half: those workers share one dependency and so are one conditional,
// while these depend on different things and several on nothing at all. The
// two analysis passes that register without a model lane each say below why a
// missing lane is not a reason to leave their work undone.
func addCapturePipelineJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) {
	// The Telegram ingest job is not periodic — a poll enqueues one per accepted
	// update in the same transaction as the raw capture row; the worker role only
	// needs the worker registered. Registered unconditionally: unlike
	// Gmail/Graph, a channel connection carries its own per-connection
	// credential (no deployment-wide OAuth app to gate on), so there is nothing
	// to check for before wiring it up.
	addDeclaredWorker[TelegramIngestArgs](reg, newTelegramIngestWorker(pool, cfg.CaptureConfig, log))
	// The captured-organization auto-enrich sweep (ADR-0072/A118): always
	// registered, it enqueues system deep reads the site worker applies.
	autoEnrich := newCaptureAutoEnrichSweepWorker(pool, log)
	addDeclaredWorker[CaptureAutoEnrichSweepArgs](reg, autoEnrich)
	addDeclaredWorker[CaptureAutoEnrichWorkspaceArgs](reg, &captureAutoEnrichWorkspaceWorker{sweeper: autoEnrich})
	// The outbound send is not periodic — the api stages one job per accepted
	// message, in the same transaction as the activity; this role only needs
	// the worker registered.
	if cfg.SendRegistry != nil {
		addDeclaredWorker[SendEmailArgs](reg, newSendWorker(pool, cfg.SendRegistry, cfg.SendPacing, cfg.SendBlob))
		// The alarm for a message a rep chose to send later. Firing one creates
		// its delivery and its dispatch job, so it registers only where that
		// machinery exists — a role that cannot send cannot fire either.
		if cfg.SendDelivery != nil {
			addDeclaredWorker[ScheduledSendArgs](reg,
				newScheduledSendWorker(pool, cfg.SendDelivery, cfg.SendBlob, cfg.SendPacing))
		}
	}

	if cfg.ClassifyBrain != nil {
		addDeclaredWorker[CaptureClassifyArgs](reg, &captureClassifyWorker{pool: pool})
		addDeclaredWorker[CaptureClassifyWorkspaceArgs](reg, &captureClassifyWorkspaceWorker{
			classifier: NewCaptureClassifier(pool, cfg.ClassifyBrain, log),
		})
	}

	if cfg.EnrichBrain != nil {
		addDeclaredWorker[CaptureEnrichArgs](reg, &captureEnrichWorker{pool: pool})
		addDeclaredWorker[CaptureEnrichWorkspaceArgs](reg, &captureEnrichWorkspaceWorker{
			enricher: NewCaptureEnricher(pool, cfg.EnrichBrain, log),
			log:      log,
		})
	}

	// Registered unconditionally: the org-name promotion weighs evidence rows
	// the enrich pass already wrote, so it needs no model. Gating it on a brain
	// would leave an AI-less deployment unable to act on signatures it had
	// already collected.
	addDeclaredWorker[OrgNamePromotionArgs](reg, &orgNamePromotionWorker{pool: pool})
	addDeclaredWorker[OrgNamePromotionWorkspaceArgs](reg, &orgNamePromotionWorkspaceWorker{promoter: NewOrgNamePromoter(pool, log)})

	// Registered unconditionally for a different reason: only the counterparty
	// verdict's JUDGING stage needs a model, and the worker skips that stage
	// when none is configured. Gating the whole worker on a brain would mean an
	// AI-less deployment never staged a review for an existing unsure row and
	// never redacted mail it had already hidden.
	addDeclaredWorker[CounterpartyVerdictArgs](reg, &counterpartyVerdictWorker{pool: pool})
	addDeclaredWorker[CounterpartyVerdictWorkspaceArgs](reg, &counterpartyVerdictWorkspaceWorker{
		engine: NewCounterpartyVerdictEngine(pool, cfg.VerdictBrain, log),
	})
	// The confidentiality verdict, registered unconditionally for a related but
	// distinct reason: a deployment with no model bound holds every thread, and
	// the RETIRING stage is what moves a thread that spent its attempts to a
	// terminal `unsure` instead of leaving it claimable forever. Gating the
	// worker on a brain would leave exactly that deployment with a backlog
	// nothing ever ends.
	addDeclaredWorker[ConfidentialityVerdictArgs](reg, &confidentialityVerdictWorker{pool: pool})
	addDeclaredWorker[ConfidentialityVerdictWorkspaceArgs](reg, &confidentialityVerdictWorkspaceWorker{
		engine: NewConfidentialityVerdictEngine(pool, cfg.ConfidentialityBrain, log),
	})
	// The trace sweep, registered unconditionally and deliberately so: it is
	// what makes the 24-hour retention true, and under the trace_payloads
	// posture that retention is a promise about message content. A deployment
	// that composed capture at all must expire it.
	addDeclaredWorker[CaptureTraceSweepArgs](reg, &captureTraceSweepWorker{pool: pool})
	addDeclaredWorker[CaptureTraceSweepWorkspaceArgs](reg, &captureTraceSweepWorkspaceWorker{pool: pool})
}

// GraphWatchConfig configures the Microsoft Graph subscription-renewal pass.
// NotificationURL is the endpoint Microsoft posts change notifications to,
// operator token and all (empty disables the pass entirely — Outlook capture
// stays on the poll); Interval is the scan cadence; and RenewWithin is how far
// ahead of a subscription's deadline it is renewed.
//
// RenewWithin matters more here than it does for Gmail: a Graph subscription
// lapses in under three days where a Gmail watch lasts seven, so a deployment
// that carried Gmail's defaults across would let every Outlook mailbox go
// quiet between scans.
type GraphWatchConfig struct {
	NotificationURL string
	Interval        time.Duration
	RenewWithin     time.Duration
}

// GmailWatchConfig configures the Gmail push-watch maintenance pass. Topic is
// the Pub/Sub topic Gmail publishes change notifications to (empty disables the
// pass entirely — capture stays on the poll); Interval is the scan cadence; and
// RenewWithin is how far ahead of a watch's expiry it is re-registered.
type GmailWatchConfig struct {
	Topic       string
	Interval    time.Duration
	RenewWithin time.Duration
}

// GmailSyncArgs schedules one DISPATCH pass: scan the fleet for due Gmail
// connections (the sidecar's backoff/pacing gate, ADR-0063) and enqueue one
// CaptureSyncArgs job per connection. The dispatcher never syncs inline —
// per-connection jobs isolate failures and kill head-of-line blocking.
type GmailSyncArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (GmailSyncArgs) Kind() string { return "gmail_sync" }

// FleetWide marks this a dispatcher: it enumerates and enqueues,
// and does no tenant work of its own (jobs.FleetWide).
func (GmailSyncArgs) FleetWide() {}

// gmailSyncWorker is the dispatcher: due-scan, then one insert per
// connection. Uniqueness on the connection id means a still-running or
// already-queued sync is not double-enqueued; only a fleet-enumeration
// failure is returned (so River retries the tick).
type gmailSyncWorker struct {
	registry *capture.Registry
	log      *slog.Logger
}

func (w *gmailSyncWorker) Work(ctx context.Context, _ *river.Job[GmailSyncArgs]) error {
	var enumErr error
	for _, desc := range w.registry.Connectors() {
		due, err := w.registry.DueConnections(ctx, desc.Name)
		if err != nil {
			enumErr = errors.Join(enumErr, err)
		}
		for _, d := range due {
			if err := dispatchOne(ctx, CaptureSyncArgs{
				Workspace:    d.Workspace.UUID,
				ConnectionID: d.ID.String(),
				Provider:     desc.Name,
			}, nil); err != nil {
				// A refused enqueue means this connection is never synced, so
				// it fails the DISPATCHER — the same posture as the watch
				// dispatcher below, which this one is the mirror of.
				enumErr = errors.Join(enumErr, fmt.Errorf("enqueueing the sync for connection %s: %w", d.ID, err))
			}
		}
	}
	return jobs.FaultContext(ctx, enumErr)
}

// CaptureSyncArgs syncs ONE connection. Unique by args while incomplete, so
// the dispatcher and the (future) push webhook can both enqueue without
// double-running a mailbox.
type CaptureSyncArgs struct {
	Workspace    ids.UUID `json:"workspace_id"`
	ConnectionID string   `json:"connection_id"`
	Provider     string   `json:"provider"`
}

// Kind is the stable job identifier River persists in river_job.
func (CaptureSyncArgs) Kind() string { return "capture_sync" }

// WorkspaceID binds this connection's sync to its tenant (jobs.WorkspaceScoped).
func (a CaptureSyncArgs) WorkspaceID() ids.UUID { return a.Workspace }

// captureSyncWorker runs one SyncOnce under the connection's workspace. A
// sync failure returns nil after the registry has recorded it: the sidecar's
// backoff owns the retry cadence (ADR-0063) — a River retry would bypass it.
type captureSyncWorker struct {
	registry *capture.Registry
	log      *slog.Logger
}

func (w *captureSyncWorker) Work(ctx context.Context, job *river.Job[CaptureSyncArgs]) error {
	conn, err := ids.Parse(job.Args.ConnectionID)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("capture_sync: connection id: %w", err))
	}
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if err := w.registry.SyncOnce(wsCtx, conn); err != nil {
		w.log.WarnContext(ctx, "capture connection sync failed",
			"connection", job.Args.ConnectionID, "provider", job.Args.Provider, "err", err)
	}
	return nil
}

// GmailWatchArgs schedules one push-watch maintenance pass: register a Gmail
// users.watch for every active connection that has none yet and renew any
// nearing its 7-day expiry (capture.md CAP-DDL-2). Scheduled only when a
// Pub/Sub topic is configured; without one, no watch job runs and capture stays
// on the poll (GmailSyncArgs).
type GmailWatchArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (GmailWatchArgs) Kind() string { return "gmail_watch_renew" }

// FleetWide marks this a dispatcher: it enumerates and enqueues,
// and does no tenant work of its own (jobs.FleetWide).
func (GmailWatchArgs) FleetWide() {}

// gmailWatchWorker walks the fleet's active Gmail connections whose watch is
// missing or nearing expiry and enqueues ONE renewal job per connection,
// against the configured Pub/Sub topic. It mirrors gmailSyncWorker — the same
// collectDue-shaped walk, keyed on the renewal deadline instead of the sync
// cursor — and it fans out at the same granularity, because a watch belongs to
// a connection rather than to a workspace: a mailbox whose renewal fails is one
// connection's problem, and per-connection jobs keep it from being read as the
// tenant's.
type gmailWatchWorker struct {
	registry    *capture.Registry
	renewWithin time.Duration
	log         *slog.Logger
}

func (w *gmailWatchWorker) Work(ctx context.Context, _ *river.Job[GmailWatchArgs]) error {
	return jobs.FaultContext(ctx, dispatchDueWatches(ctx, w.registry, providerGmail, w.renewWithin,
		func(ws ids.UUID, connID string) error {
			return dispatchOne(ctx, GmailWatchRenewArgs{Workspace: ws, ConnectionID: connID}, nil)
		}))
}

// dispatchDueWatches is the fleet walk both providers' watch dispatchers run:
// every connection whose subscription is missing or nearing its deadline gets
// ONE renewal job. The provider and the Args type are all that differ, and a
// second copy of the walk is the copy that would stop joining the enqueue
// error.
func dispatchDueWatches(
	ctx context.Context, reg *capture.Registry, provider string,
	renewWithin time.Duration, enqueue func(ids.UUID, string) error,
) error {
	// Returns the bare cause: each caller is a River Work method, and the gate
	// that keeps a raw cause out of river_job.errors reads the return AT that
	// method — so the wrap belongs there, where a reader of the worker sees it.
	due, enumErr := reg.DueWatches(ctx, provider, renewWithin)
	for _, d := range due {
		if err := enqueue(d.Workspace.UUID, d.ID.String()); err != nil {
			// A refused enqueue means this connection's watch is never renewed,
			// so it fails the DISPATCHER rather than being logged past.
			enumErr = errors.Join(enumErr, fmt.Errorf("enqueueing the watch renewal for connection %s: %w", d.ID, err))
		}
	}
	return enumErr
}

// GmailWatchRenewArgs renews ONE connection's push watch. The workspace travels
// with it because capture_connection reads are workspace-predicated and a job
// carries no session.
type GmailWatchRenewArgs struct {
	Workspace    ids.UUID `json:"workspace_id"`
	ConnectionID string   `json:"connection_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (GmailWatchRenewArgs) Kind() string { return "gmail_watch_renew_connection" }

// WorkspaceID binds this renewal to its tenant (jobs.WorkspaceScoped).
func (a GmailWatchRenewArgs) WorkspaceID() ids.UUID { return a.Workspace }

// gmailWatchRenewWorker renews one connection's watch and advances
// watch_expires_at. A revoked mailbox fails its OWN row now, where before it
// was logged and skipped inside a pass River recorded as completed.
type gmailWatchRenewWorker struct {
	registry *capture.Registry
	topic    string
}

func (w *gmailWatchRenewWorker) Work(ctx context.Context, job *river.Job[GmailWatchRenewArgs]) error {
	return jobs.FaultContext(ctx, renewOneWatch(ctx, w.registry, job.Args, job.Args.ConnectionID, w.topic))
}

// renewOneWatch is the body both providers' renewal workers run: bind the
// tenant the job carries, read the connection id it names, and let the registry
// perform the provider call and advance watch_expires_at.
//
// The topic is whatever that provider's Watch takes — a Pub/Sub topic for
// Gmail, a notification URL for Graph — and this never looks inside it.
//
// Returns the bare cause, for the reason dispatchDueWatches does.
func renewOneWatch(
	ctx context.Context, reg *capture.Registry, args jobs.WorkspaceScoped, connectionID, topic string,
) error {
	wsCtx, err := workspaceJobCtx(ctx, args)
	if err != nil {
		return err
	}
	connID, err := ids.Parse(connectionID)
	if err != nil {
		return fmt.Errorf("%s: connection id: %w", args.Kind(), err)
	}
	return reg.RenewWatch(wsCtx, connID, topic)
}

// GraphWatchArgs schedules one subscription-maintenance pass: register a Graph
// change-notification subscription for every active Outlook connection that has
// none yet, and renew any nearing its deadline. Scheduled only when a
// notification URL is configured; without one, no subscription job runs and
// Outlook capture stays on the poll.
type GraphWatchArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (GraphWatchArgs) Kind() string { return graphWatchKind }

// graphWatchKind is the kind string, named so the registration can be read from
// api/jobs.yaml without constructing the args — a dispatcher literal outside
// periodicFor is what jobdispatcherenqueue_test refuses, and rightly: building
// one to ask a question looks exactly like building one to enqueue it.
const graphWatchKind = "graph_watch_renew"

// FleetWide marks this a dispatcher: it enumerates and enqueues, and does no
// tenant work of its own (jobs.FleetWide).
func (GraphWatchArgs) FleetWide() {}

// graphWatchWorker is the Graph twin of gmailWatchWorker, on the same walk.
type graphWatchWorker struct {
	registry    *capture.Registry
	renewWithin time.Duration
	log         *slog.Logger
}

func (w *graphWatchWorker) Work(ctx context.Context, _ *river.Job[GraphWatchArgs]) error {
	return jobs.FaultContext(ctx, dispatchDueWatches(ctx, w.registry, providerGraph, w.renewWithin,
		func(ws ids.UUID, connID string) error {
			return dispatchOne(ctx, GraphWatchRenewArgs{Workspace: ws, ConnectionID: connID}, nil)
		}))
}

// GraphWatchRenewArgs renews ONE connection's subscription. The workspace
// travels with it because capture_connection reads are workspace-predicated and
// a job carries no session.
type GraphWatchRenewArgs struct {
	Workspace    ids.UUID `json:"workspace_id"`
	ConnectionID string   `json:"connection_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (GraphWatchRenewArgs) Kind() string { return "graph_watch_renew_connection" }

// WorkspaceID binds this renewal to its tenant (jobs.WorkspaceScoped).
func (a GraphWatchRenewArgs) WorkspaceID() ids.UUID { return a.Workspace }

// graphWatchRenewWorker renews one connection's subscription and advances
// watch_expires_at. A revoked mailbox fails its OWN row, where a pass that
// logged and skipped would be recorded as completed.
type graphWatchRenewWorker struct {
	registry        *capture.Registry
	notificationURL string
}

func (w *graphWatchRenewWorker) Work(ctx context.Context, job *river.Job[GraphWatchRenewArgs]) error {
	return jobs.FaultContext(ctx, renewOneWatch(ctx, w.registry, job.Args, job.Args.ConnectionID, w.notificationURL))
}
