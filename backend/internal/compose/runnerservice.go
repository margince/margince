// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Surface-B runner, assembled: catalog seeding, job claiming, run
// execution and approval-decision resume — composed here because the
// pieces span three modules (agents/runner drives, identity resolves
// the passport, ai routes the brain) that never import each other.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/overlay"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

// RunWallClock is the §4 wall-clock guarantee (RATIFY default 15 min):
// the third bound alongside steps and output tokens.
const RunWallClock = 15 * time.Minute

// claimBatch bounds how many due jobs one tick executes per workspace.
const claimBatch = 4

// RunnerService drives scheduled Surface-B runs. It is the WORKER's
// entry point: TickWorkspace seeds + executes one tenant's due jobs,
// HandleEvent resumes suspended runs when their approval is decided.
type RunnerService struct {
	// pool reads the installation's base language, which a run's final summary
	// is written in. A summary is filed on a record the whole team reads, so it
	// takes the shared language rather than any one reader's.
	pool      *pgxpool.Pool
	store     *runner.Store
	runner    *runner.Runner
	identity  *identity.Service
	retriever retrieval.Retriever
	log       *slog.Logger
	// specByName resolves a stored job's catalog entry. It defaults to
	// ScheduledAgentSpecByName — the catalog IS code and this does not make it
	// configuration. What that default adds is the declared allowlist: a
	// resolver that returned the runner's bare entry would hand back empty
	// Tools, which is read as no narrowing, and the run would be bounded by
	// its passport alone.
	//
	// It is a seam because the integration lane needs a spec that can stage
	// an approval, and no shipped agent has one: every tool both catalog
	// entries name is auto-execute, so the runner's suspend/approve/resume
	// path is unreachable from the catalog. Before the allowlist bound, that
	// path was reached by driving the sweep to archive_record — an action its
	// own goal forbids — so the coverage was only ever there by way of a
	// prohibited call. Rather than lose it, the lane reaches for the wiring.
	specByName func(string) (runner.AgentSpec, bool)
}

// RunnerOption adjusts a RunnerService at construction.
type RunnerOption func(*RunnerService)

// WithSpecResolver replaces the catalog lookup. ONLY the integration lane
// passes it: production always resolves against ScheduledAgentSpecByName, and
// a deployment cannot reach this.
//
// A resolver passed here OWNS the allowlist for every name it answers,
// including the shipped agents. Fall back to ScheduledAgentSpecByName for
// those rather than to the runner catalog, whose entries carry no tools.
func WithSpecResolver(resolve func(string) (runner.AgentSpec, bool)) RunnerOption {
	return func(s *RunnerService) { s.specByName = resolve }
}

// NewRunnerService assembles the runner over the SAME governed registry
// every other agent surface dispatches through — the two-directions
// invariant is a property of this constructor: there is no other
// registry to hand it. resolveIncumbent is the per-workspace live-incumbent
// resolver the overlay write-back path reaches HubSpot through when a
// Surface-B run's agent tool writes a record; the worker passes a FromEnv
// vault-backed resolver, and nil degrades write-back to errNoWriteIncumbent
// (reads and non-SoR tools are unaffected).
func NewRunnerService(pool *pgxpool.Pool, brain runner.Brain, draftBrain completer, retriever retrieval.Retriever, log *slog.Logger, resolveIncumbent func(context.Context) (overlay.Incumbent, error), send SendPath, opts ...RunnerOption) *RunnerService {
	svc := &RunnerService{
		pool:       pool,
		store:      runner.NewStore(InstallationDB(pool)),
		runner:     runner.New(registryWithDraftBrain(pool, draftBrain, resolveIncumbent, send), brain),
		identity:   identity.NewService(pool),
		specByName: ScheduledAgentSpecByName,
		retriever:  retriever,
		log:        log,
	}
	// Read the join HERE, before any option can replace the resolver and before
	// a worker goroutine could be the first to touch it: a contract that
	// disagrees with the catalog must refuse to start, not fail tick after tick
	// inside River's panic recovery.
	mustScheduledAgents()
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// Tick is the installation's scheduler pass: close the runs abandoned since
// last time, seed the catalog occurrences due at now, then execute the jobs it
// can claim. ctx must already carry the workspace — the caller binds it, so
// this pass can only ever touch the tenant it was handed.
//
// Three failures, three different destinations, because each belongs to a
// different row. A seeding or claiming failure is returned, and returning it is
// the point: it is this tenant's pass that could not run, and its own job row is
// where that has to land. Execution failures do NOT come back here — executeJob
// records each on the job row it belongs to, because a brief that never ran must
// say why on the row an operator reads, not take the whole pass down with it. The
// sweep's own failure is only logged: it owns no row of this pass, and a tenant's
// schedule must still run when the accounting for last week's crash cannot.
func (s *RunnerService) Tick(ctx context.Context, now time.Time) error {
	now = now.UTC()
	// The pass's own identity, bound once for everything it does.
	//
	// It is not decoration: seeding, sweeping and finishing all announce their
	// occurrence to the AI-activity projection now, and an announcement carries
	// the write shape — a ledger row and an outbox row, both of which take their
	// actor from the context. A pass with no actor bound could not write either,
	// and the rail would silently never learn that the 06:00 brief was queued.
	//
	// Refused BEFORE that binding if somebody else is already on the context.
	// Checking after it would be decoration: schedulerContext overwrites the
	// actor, so a human, an agent or a buyer handed to Tick would be converted
	// to the system principal and then admitted by the very check meant to
	// refuse them. The pass runs on the installation's own authority, and the
	// only caller entitled to start it is one carrying nobody.
	if actor, ok := principal.Actor(ctx); ok && actor.Type != principal.PrincipalSystem {
		return fmt.Errorf("runner: a scheduled pass runs as the system, not as %s: %w",
			actor.Type, apperrors.ErrPermissionDenied)
	}
	ctx = schedulerContext(ctx)
	// And the binding it just made, stated as a check so the entry-point gate
	// sees an admission and a future edit to schedulerContext cannot quietly
	// stop making one.
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	s.reapAbandonedRuns(ctx)
	// Seeding failures are collected, NOT returned here. Claiming is what makes
	// already-queued work run, and it is independent of whether tonight's
	// seeding succeeded — so returning early would let one seat whose seeding
	// keeps failing stop every other seat's queued brief from executing, on
	// every tick, for as long as the fault lasts. The pass still reports the
	// failure at the end, on its own job row, which is where an operator reads
	// it.
	var seeding []error
	for _, spec := range mustScheduledAgents() {
		if due := spec.DueAt(now); !now.Before(due) {
			if err := s.seedSeats(ctx, spec, now, due); err != nil {
				seeding = append(seeding, err)
			}
		}
	}
	jobs, err := s.store.ClaimDueJobs(ctx, claimBatch)
	if err != nil {
		return errors.Join(append(seeding, err)...)
	}
	for _, job := range jobs {
		s.executeJob(ctx, job)
	}
	return errors.Join(seeding...)
}

// seedSeats queues tonight's occurrence of one spec for every rep who granted
// it, each job carrying that rep's own passport.
//
// WHY THE GRANTS DRIVE THE LOOP. An agent run acts as a contact, and the only
// credential it may act with is one that colleague minted for themselves. There
// is no workspace-wide authority to fall back on and deliberately so, so a
// spec with no live grants has nothing to run tonight — that is a workspace
// where nobody has said yes yet, not a fault, and it queues nothing.
//
// A rep whose seeding fails does not cost the others theirs. The loop records
// the failure and carries on, then returns them joined: stopping at the first
// would leave a whole team unseeded because one seat's row was bad, and the
// next tick would arrive at the same seat first and stop there again.
func (s *RunnerService) seedSeats(ctx context.Context, spec runner.AgentSpec, now, due time.Time) error {
	grants, err := s.store.LiveGrantsFor(ctx, spec.Name)
	if err != nil {
		return fmt.Errorf("read who granted %s: %w", spec.Name, err)
	}
	var failures []error
	for _, grant := range grants {
		// Live() is the reader's guarantee, asserted rather than assumed: the
		// passport is what authorizes the run, and a job seeded against a dead
		// one is a run that can only fail in the small hours with nobody
		// watching. The query already filters for this; the check is here so
		// that a later edit loosening the query cannot quietly widen the
		// fan-out to credentials that stopped working.
		if !grant.Live() || grant.PassportID == nil {
			continue
		}
		passportID := *grant.PassportID
		if err := s.store.EnqueueJob(ctx, spec.Name, spec.TriggerRef(now, grant.UserID), &passportID, due); err != nil {
			failures = append(failures, fmt.Errorf("seat %s: %w", grant.UserID, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("seeding %s for %d of %d seats: %w",
			spec.Name, len(failures), len(grants), errors.Join(failures...))
	}
	return nil
}

// schedulerActor is who the scheduling pass is. The runs it seeds are executed
// under their own passports later; this names only the pass that placed them.
const schedulerActor = "system:agent_scheduler"

// resumeActor is who closes or resumes a parked run when an approval is
// decided. The resumed leg itself runs under the run's own passport; this names
// only the consumer that reacted to the decision.
const resumeActor = "system:agent_resume"

// schedulerContext binds the pass's actor and one correlation id, so every row
// a single tick writes groups under the tick that wrote it.
func schedulerContext(ctx context.Context) context.Context {
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: schedulerActor,
	})
}

// stuckRunGrace is how far past its wall clock a 'running' row must be before
// the sweep calls it abandoned.
//
// TWICE RunWallClock, not once, because updated_at is stamped when the run starts
// and nothing bumps it while the loop runs: a live run's row ages at wall-clock
// speed, so a run that spends its whole budget is already a full RunWallClock old
// when it writes its outcome. One wall clock of grace would put that write inside
// the window. SaveOutcome would then correct the status — it guards on id, not on
// status — but an operator had already been told a run was abandoned, and for a
// resumed run that is a lie about a mutation a human approved.
const stuckRunGrace = 2 * RunWallClock

// abandonedRunReason states what the sweep OBSERVED, and never why. The predicate
// is a status and an age; it cannot tell a died-mid-resume from a first-leg run
// whose process was killed, so naming either mechanism would put an unverifiable
// claim on the only record an operator gets. What it can say is the part that
// changes what they do next: the run's tools write as they execute, and trace is
// only persisted at SaveOutcome, so a swept row is empty and yet its writes may
// well have landed.
const abandonedRunReason runner.FailureReason = "abandoned: still running past twice the run wall clock, so no process is " +
	"coming back for it. Its tools may already have written — check the audit log for this run id " +
	"before assuming nothing landed."

// reapAbandonedRuns closes the runs nothing will ever finish, in the tenant ctx
// is bound to. The invariant that makes them unrecoverable belongs to
// runner.Store.FailStuckRuns; what is decided HERE is only where the sweep runs
// and what happens when it fails.
//
// It rides the scheduling pass because that is the tenant loop that already
// exists and is already bound to one workspace. Best-effort inside it: a sweep
// failure must not take down the pass, whose actual job is this tenant's
// schedule. It is reported instead, because a sweep that keeps failing means runs
// are piling up in 'running' where nothing will read them.
func (s *RunnerService) reapAbandonedRuns(ctx context.Context) {
	swept, err := s.store.FailStuckRuns(ctx, stuckRunGrace, abandonedRunReason)
	if err != nil {
		s.log.Error("runner: sweeping abandoned runs", "err", err)
		return
	}
	if len(swept) > 0 {
		// The ids, not just a count: each of these may be a run a human approved
		// and never saw the end of, and an operator who cannot name one cannot go
		// read its audit trail to find out whether its writes landed. One line
		// carrying them all rather than a line each, because the occasion for this
		// log is a crash that stranded everything at once.
		s.log.Warn("runner: closed abandoned runs",
			"count", len(swept), "runs", swept, "stale_for", stuckRunGrace)
	}
}

// executeJob runs one claimed job to its outcome. Failures land on the
// job row — a brief that never ran must say why, not vanish.
func (s *RunnerService) executeJob(ctx context.Context, job runner.QueuedJob) {
	spec, known := s.specByName(job.SpecName)
	if !known {
		s.finishJob(ctx, job.ID, nil, fmt.Sprintf("agent spec %q is not in the catalog", job.SpecName))
		return
	}
	if job.PassportID == nil {
		s.finishJob(ctx, job.ID, nil,
			"no passport bound: mint one (POST /v1/passports) and bind it to the job before the run can act")
		return
	}
	agentIdentity, err := s.identity.AuthenticateAgentByID(ctx, *job.PassportID)
	if err != nil {
		// The cause goes to the operator, never onto the row: this reason is
		// announced to the AI-activity rail, where an ordinary rep reads it, and
		// identity's own message names internals they cannot act on.
		s.log.Warn("runner: a job's passport would not resolve",
			"job", job.ID, "trigger_ref", job.TriggerRef, "cause", err)
		s.finishJob(ctx, job.ID, nil, string(runner.FailureCouldNotStart))
		return
	}
	// One correlation id per run: every event the run's writes emit
	// groups under it (events.md — "one originating request/agent-run").
	runCtx := principal.WithCorrelationID(principal.WithActor(ctx, agentIdentity.Principal()), ids.NewV7())

	runID, created, err := s.store.StartRun(runCtx, spec, job.TriggerRef, *job.PassportID)
	if err != nil {
		// Same rule as the resolution failure above: a write error carries the
		// driver's words, which must not reach the column a rep reads.
		s.log.Warn("runner: a run row could not be written",
			"job", job.ID, "trigger_ref", job.TriggerRef, "cause", err)
		s.finishJob(ctx, job.ID, nil, string(runner.FailureCouldNotStart))
		return
	}
	if !created {
		// This occurrence already ran (or is suspended) — the job was a
		// duplicate trigger and idempotency absorbed it.
		s.finishJob(ctx, job.ID, nil, "")
		return
	}
	// Every ai_call the run's model lane makes stamps this run — the
	// trace that ties a routed model call back to the Surface-B run it
	// served.
	runCtx = principal.WithAgentRunID(runCtx, runID)

	bounded, cancel := context.WithTimeout(runCtx, RunWallClock)
	defer cancel()
	// Grounding runs under the run's OWN deadline: RunWallClock has to bound
	// everything the run does, or it does not bound the run. On the retriever's
	// own timeout it would otherwise age the row toward the abandoned sweep while
	// the loop had not started a single step — and grounding already degrades to
	// an ungrounded run rather than failing one.
	grounding := s.seedGrounding(bounded, spec.Goal)
	res, err := s.runner.Run(bounded, runner.Job{
		Goal:       spec.Goal,
		TriggerRef: job.TriggerRef,
		Budget:     spec.Budget,
		Tools:      spec.Tools,
		Grounding:  grounding,
		// The rendered rule, not a code: the runner is a module and may not
		// import compose, where the one spelling of this block lives.
		LanguageRule: promptlang.Rule(identity.BaseLanguageForPrompt(bounded, s.pool)),
	})
	s.landOutcome(runCtx, runID, job.TriggerRef, res, err)
	s.finishJob(ctx, job.ID, &runID, "")
}

func (s *RunnerService) landOutcome(ctx context.Context, runID ids.UUID, triggerRef string, res runner.Result, runErr error) {
	if runErr != nil {
		s.log.Error("runner: a run faulted outside its own degrade path",
			"trigger_ref", triggerRef, "run", runID, "cause", runErr)
		if err := s.store.MarkFailed(ctx, runID, runner.FailureRunFaulted); err != nil {
			s.log.Error("runner: marking run failed", "run", runID, "err", err)
		}
		return
	}
	if err := s.store.SaveOutcome(ctx, runID, res); err != nil {
		s.log.Error("runner: saving outcome", "run", runID, "err", err)
	}
}

func (s *RunnerService) finishJob(ctx context.Context, jobID ids.UUID, runID *ids.UUID, failReason string) {
	if failReason != "" {
		s.log.Warn("runner: job failed", "job", jobID, "reason", failReason)
	}
	if err := s.store.FinishJob(ctx, jobID, runID, failReason); err != nil {
		s.log.Error("runner: finishing job", "job", jobID, "err", err)
	}
}

// seedGrounding retrieves T2 seed context for the run's goal under the
// AGENT's own principal — the run grounds on exactly what its passport
// may see, and a retrieval failure degrades to an ungrounded run
// rather than blocking the brief.
func (s *RunnerService) seedGrounding(ctx context.Context, goal string) []runner.Grounding {
	if s.retriever == nil {
		return nil
	}
	found, err := s.retriever.Search(ctx, retrieval.Query{Text: goal, Limit: 5})
	if err != nil {
		s.log.Warn("runner: seed retrieval failed — running ungrounded", "err", err)
		return nil
	}
	// The ranking's kind is deliberately not threaded into the grounding: a
	// seed enters the run at T2 whichever lane surfaced it, and the run's own
	// answer carries that tier. A lexically-ranked seed is a less RELEVANT
	// seed, not a less trustworthy one.
	grounding := make([]runner.Grounding, 0, len(found.Hits))
	for _, hit := range found.Hits {
		for _, ev := range hit.Evidence {
			grounding = append(grounding, runner.Grounding{
				SourceID:  ev.Source,
				TrustTier: "T2",
				Content:   ev.Snippet,
			})
		}
	}
	return grounding
}
