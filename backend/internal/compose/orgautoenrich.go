// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The prompt half of captured-organization auto-enrich: an organization
// event queues the workspace's enrich pass NOW.
//
// The daily sweep (captureautoenrich.go) is the reconciler, and before this
// consumer it was also the only trigger for a company a person or an agent
// CREATED — so one minted five minutes after the sweep waited a day for its
// dossier, which is exactly the moment its creator is looking at the empty
// page. THE TRIGGER IS THE EVENT, NOT THE WRITER (the person-auto-enrich
// rule): organization.created and organization.updated reach the outbox
// because the write shape puts them there, so manual entry, the MCP tools,
// a site-read confirm and an import all land here without any of them
// knowing this consumer exists.
//
// It queues the WORKSPACE PASS, not a read for the event's organization.
// The pass owns the gates — the daily budget, the cursor backoff, the
// dossier check, and the auto-enrich setting for the enrich half (its
// domain-triage half deliberately runs whatever the setting says; see
// sweepWorkspace) — and re-derives which organizations are due, so this
// consumer cannot become a second spelling of that decision, and a burst
// of creates collapses onto one queued pass.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// orgAutoEnrichFreshWindow is how old an organization event may be and still
// queue a pass. This consumer exists for PROMPTNESS alone — the daily sweep
// already owns everything older — and the bound is what makes first
// deployment safe: a new consumer group is created at stream position 0
// (events.Subscriber), so the first boot replays the organization stream's
// whole history into this handler, and without the bound every historical
// event would mint a job. An hour is generous for a live event's delivery
// lag (the relay ships in seconds) while excluding any replayed backlog.
const orgAutoEnrichFreshWindow = time.Hour

// OrgAutoEnrichTrigger queues one auto-enrich workspace pass per
// organization event.
type OrgAutoEnrichTrigger struct {
	pool    *pgxpool.Pool
	enqueue *jobs.Runner
	log     *slog.Logger
}

// NewOrgAutoEnrichTrigger builds the trigger over an insert-only jobs runner.
func NewOrgAutoEnrichTrigger(pool *pgxpool.Pool, enqueue *jobs.Runner, log *slog.Logger) *OrgAutoEnrichTrigger {
	return &OrgAutoEnrichTrigger{pool: pool, enqueue: enqueue, log: log}
}

// HandleEvent routes one envelope. An event this consumer does not care about
// answers nil, so the group keeps flowing rather than wedging on somebody
// else's traffic. An enqueue failure comes back as an error and the bus
// redelivers — safe, because a redelivered event dedupes onto the pass the
// first delivery queued, and the sweep still reconciles whatever slips
// through.
func (g *OrgAutoEnrichTrigger) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Entity.Type != string(recordTypeOrganization) {
		return nil
	}
	switch env.Type {
	// Every event that can make an organization newly due: appearing,
	// gaining a domain (a company save's domain change arrives as
	// organization.updated), or inheriting one in a merge. An archive needs
	// no reaction — the pass only ever starts reads, and an archived
	// organization is not due.
	case "organization.created", "organization.updated", "organization.merged":
	default:
		return nil
	}
	// A stale event has no promptness left to buy: it is either a replayed
	// backlog (a freshly created consumer group starts at stream position 0)
	// or a delivery the bus held for hours, and in both cases the daily
	// sweep's next pass already covers whatever it announced. Skipping is
	// nil, not an error — erroring would redeliver the same stale event.
	if time.Since(env.OccurredAt) > orgAutoEnrichFreshWindow {
		return nil
	}
	// The uniqueness is the flood bound: any authenticated writer can emit
	// organization.updated in a loop (the store's emit has no value-changed
	// guard), and without it every event would land its own row on the
	// shared default queue. ByArgs over the active states collapses a burst
	// onto the one pass already queued or running for this workspace.
	//
	// The states include running because River requires it in any custom
	// list, and that opens the one hole this trigger accepts: a company
	// created while a pass is mid-run can be deduped against a pass that
	// listed the due organizations before the new row landed, and then
	// waits for the reconciling sweep instead of being read promptly. The
	// alternative — no uniqueness — trades that bounded promptness miss for
	// an unbounded queue, which is the worse failure. Same args and states
	// as the fleet dispatcher's insert (workspaceSweepOpts), so the two
	// doors dedupe against each other instead of stacking; the sweep-tag
	// gauges may therefore occasionally count a day's coverage against a
	// trigger-queued pass that did the identical work untagged.
	// The PASS, not a per-workspace child: the child kind is gone with the
	// fan-out (ADR-0103). Its args carried the workspace, so ByArgs deduped one
	// trigger against another for the SAME workspace; the pass carries none, so
	// the same dedupe now means one pending sweep at a time — which is what
	// this trigger wanted in the first place. The pass walks the workspaces
	// itself, so `ws` is no longer named: a pass over the installation cannot
	// be aimed at one tenant.
	child := CaptureAutoEnrichSweepArgs{}
	opts := oneOffPassOpts(child.Kind())
	opts.UniqueOpts = river.UniqueOpts{ByArgs: true, ByState: activeSweepStates}
	return g.enqueue.Enqueue(ctx, child, opts)
}
