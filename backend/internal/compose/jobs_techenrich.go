// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The technical lookup's two jobs: one company, and the pass that finds the
// companies due one.
//
// The sweep exists for a reason geocoding does not have. Geocoding fires when
// an address is WRITTEN, and an address only changes when somebody writes it.
// A company's mail provider changes at the COMPANY, and nothing in this
// product is written when it does — so no trigger can fire, and a picture that
// is never re-read is a picture that silently rots. Freshness is the feature,
// so something has to come back round.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// technicalLookupQueue is the pool this lane drains on.
	technicalLookupQueue = "technical_lookup"
	// technicalLookupMaxWorkers mirrors api/jobs.yaml. One, for the politeness
	// reason the declaration states, and the census holds the two equal.
	technicalLookupMaxWorkers = 1
)

// TechnicalEnrichOrganizationArgs is one queued lookup: the tenant and the
// company.
//
// The DOMAIN is not among them, deliberately, for the reason the geocode job
// keeps the address out of its own args and one more besides. The worker reads
// it from the record when it runs, so a lookup queued before an edit reads the
// domain the company actually has — and, because the args cannot carry one,
// there is no way to point this lane at a domain the record never held.
type TechnicalEnrichOrganizationArgs struct {
	Workspace      ids.UUID `json:"workspace_id"`
	OrganizationID ids.UUID `json:"organization_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (TechnicalEnrichOrganizationArgs) Kind() string { return "technical_enrich_organization" }

// WorkspaceID binds this lookup to its tenant (jobs.WorkspaceScoped).
func (a TechnicalEnrichOrganizationArgs) WorkspaceID() ids.UUID { return a.Workspace }

// TechnicalEnrichBackfillArgs is one pass over the companies due a lookup.
//
// It carries NO workspace, like GeocodeBackfillArgs and for the same reason: a
// periodic insert has no tenant to name, and the tenant belongs on the
// per-company job this pass queues.
type TechnicalEnrichBackfillArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (TechnicalEnrichBackfillArgs) Kind() string { return "technical_enrich_backfill" }

// InsertOpts carries the attempt cap the declaration publishes, because the
// periodic insert supplies uniqueness and no attempt policy of its own.
func (TechnicalEnrichBackfillArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       technicalLookupQueue,
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	}
}

// technicalInsertOpts is what a per-company lookup is queued with.
//
// Deduplicated by args while queued or running, so a rep pressing the button on
// a company the sweep just nominated joins that lookup rather than starting a
// second one against the same three services.
func technicalInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue:      technicalLookupQueue,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	}
}

// technicalBackfillOpts is technicalInsertOpts with the sweep's own PRIORITY.
//
// One worker drains this queue, so a batch of nominations is a long queue — and
// without this a rep who presses the button waits behind every one of them for
// a lookup they are watching for. River fetches every priority-1 job before any
// priority-2 job, so a human's request always jumps the backlog.
func technicalBackfillOpts() *river.InsertOpts {
	opts := technicalInsertOpts()
	opts.Priority = river.PriorityDefault + 1
	return opts
}

// addTechnicalEnrichJobs registers both kinds and returns the sweep's schedule.
//
// A deployment with no enricher registers NOTHING for the sweep: queueing rows
// nobody can work is worse than leaving the technical picture unread, which is
// an honest state. The per-company kind still registers so a request against
// such a deployment fails visibly rather than queuing forever.
func addTechnicalEnrichJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig) []*river.PeriodicJob {
	if cfg.TechnicalEnricher == nil {
		return nil
	}
	worker := &technicalEnrichWorker{pool: pool, enricher: cfg.TechnicalEnricher}
	addDeclaredWorker[TechnicalEnrichOrganizationArgs](reg, worker)
	addDeclaredWorker[TechnicalEnrichBackfillArgs](reg, &technicalBackfillWorker{pool: pool})
	return periodicFor(cfg, TechnicalEnrichBackfillArgs{})
}

// technicalEnrichWorker reads one company's public technical profile.
type technicalEnrichWorker struct {
	pool     *pgxpool.Pool
	enricher *TechnicalEnricher
}

// Work reads the three lanes and applies what completed.
//
// The lane ledger is written even when a lane failed — especially then: it is
// what carries the backoff, and a failure nobody recorded is a failure the
// sweep retries at full rate.
func (w *technicalEnrichWorker) Work(ctx context.Context, job *river.Job[TechnicalEnrichOrganizationArgs]) error {
	orgID := ids.From[ids.OrganizationKind](job.Args.OrganizationID)
	wsCtx := technicalActor(principal.WithWorkspaceID(ctx, job.Args.Workspace))
	store := people.NewStore(database.Bind(w.pool, func(context.Context) (ids.WorkspaceID, error) {
		return ids.From[ids.WorkspaceKind](job.Args.Workspace), nil
	}))

	domain, ok, err := store.TechnicalDomain(wsCtx, orgID)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if !ok {
		// Nothing to look up. Not a failure: the record carries no domain, and
		// the next pass will skip it for the same reason.
		return nil
	}

	read, outcomes := w.enricher.Read(wsCtx, orgID, domain)
	if err := store.ApplyTechnicalEnrichment(wsCtx, read, technicalChangeRecorder()); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return w.recordOutcomes(ctx, wsCtx, store, orgID, read, outcomes)
}

// recordOutcomes writes each lane's verdict to the ledger.
func (w *technicalEnrichWorker) recordOutcomes(
	ctx, wsCtx context.Context, store *people.Store, orgID ids.OrganizationID,
	read people.TechnicalEnrichment, outcomes []laneOutcome,
) error {
	now := read.ObservedAt
	for _, outcome := range outcomes {
		verdict := technicalVerdict(outcome, read)
		if err := store.RecordTechnicalLane(wsCtx, orgID, outcome.Lane, verdict, now); err != nil {
			return jobs.FaultContext(ctx, err)
		}
		if outcome.Err != nil {
			// Logged rather than returned: one lane failing must not fail the
			// job and undo the two that worked. The ledger carries the retry.
			slog.WarnContext(ctx, "a technical lookup lane did not complete",
				"lane", outcome.Lane, "organization", orgID.String(), "error", outcome.Err)
		}
	}
	return nil
}

// technicalVerdict reads one lane's outcome in the ledger's vocabulary.
func technicalVerdict(outcome laneOutcome, read people.TechnicalEnrichment) string {
	if !outcome.Completed {
		return people.TechnicalOutcomeFailed
	}
	if outcome.Refused {
		return people.TechnicalOutcomeRefused
	}
	for _, observation := range read.Observations {
		if people.LaneOwningField(observation.Field) == outcome.Lane {
			return people.TechnicalOutcomeApplied
		}
	}
	// Completed with nothing: an authoritative empty answer, which is a fact
	// about the company rather than a gap in what we asked.
	return people.TechnicalOutcomeEmpty
}

// technicalBackfillWorker nominates the companies due a lookup.
type technicalBackfillWorker struct{ pool *pgxpool.Pool }

// Work nominates up to TechnicalBackfillBatch companies per workspace.
func (w *technicalBackfillWorker) Work(ctx context.Context, _ *river.Job[TechnicalEnrichBackfillArgs]) error {
	workspaces, err := enumerateWorkspaces(ctx, w.pool)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	queued := 0
	for _, ws := range workspaces {
		n, err := w.sweepOneWorkspace(ctx, ws)
		if err != nil {
			return jobs.FaultContext(ctx, err)
		}
		queued += n
	}
	if queued == 0 {
		return nil
	}
	// Said out loud because the pace is the surprising part: one worker against
	// services paced in seconds means a batch is minutes of work, and an
	// operator watching for technical profiles should know that is normal.
	slog.InfoContext(ctx, "technical lookup queued a batch",
		"companies", queued, "batch", people.TechnicalBackfillBatch)
	return nil
}

// sweepOneWorkspace nominates one tenant's due companies.
func (w *technicalBackfillWorker) sweepOneWorkspace(ctx context.Context, ws ids.UUID) (int, error) {
	// Reads under an actor of its own: nothing queued this on a person's
	// behalf — the installation is asking which of its own companies it has
	// not looked at lately — so it names itself rather than borrowing a
	// principal, and organization:read is gated like any other read.
	wsCtx := technicalBackfillActor(principal.WithWorkspaceID(ctx, ws))
	store := people.NewStore(database.Bind(w.pool, func(context.Context) (ids.WorkspaceID, error) {
		return ids.From[ids.WorkspaceKind](ws), nil
	}))
	due, err := store.ListTechnicalDue(wsCtx, people.TechnicalBackfillBatch, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	if len(due) == 0 {
		return 0, nil
	}
	client, err := river.ClientFromContextSafely[pgx.Tx](wsCtx)
	if err != nil {
		return 0, err
	}
	for _, orgID := range due {
		if _, err := client.Insert(wsCtx, TechnicalEnrichOrganizationArgs{
			Workspace:      ws,
			OrganizationID: orgID.UUID,
		}, technicalBackfillOpts()); err != nil {
			return 0, err
		}
	}
	return len(due), nil
}

// technicalActor names the lookup itself as the writer.
//
// A system principal rather than a borrowed one, so an audit row says the
// installation went and read public records rather than leaving a gated write
// with nobody behind it.
func technicalActor(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:technical-lookup",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// technicalBackfillActor is the sweep's own reader.
func technicalBackfillActor(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:technical-backfill",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// TechnicalEnrichmentConfig is the sweep's cadence.
type TechnicalEnrichmentConfig struct {
	// BackfillInterval is how often the pass looks for companies whose
	// technical picture is missing or stale. It runs on start too, so wiring
	// the lookup on a database that already holds its customers begins reading
	// them at boot rather than at the first tick.
	BackfillInterval time.Duration
}
