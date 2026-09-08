// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The prompt half of signature enrichment: captured mail queues the
// workspace's enrich pass NOW.
//
// The nightly pass (captureenrich.go) is the reconciler, and before this
// consumer it was also the only trigger — so a contact who wrote this morning
// had their details read tonight, which is exactly the wrong way round. A rep
// opens the record right after the mail lands, and that is when it should be
// current.
//
// THE TRIGGER IS THE EVENT, NOT THE WRITER, the same rule the organization
// trigger states: activity.captured reaches the outbox because the write shape
// puts it there, so every connector and every ingest path lands here without
// knowing this consumer exists.
//
// It queues the WORKSPACE PASS rather than a read for this one message. The
// pass owns the gates — the per-mailbox setting, the model budget, the read
// watermark — and re-derives who is due, so this consumer cannot become a
// second spelling of that decision and a burst of mail collapses onto one job.
//
// THREE DOORS, because mail arriving is not the only way somebody becomes
// readable. The pass selects a person joined to their own open inbound mail, so
// either half of that pair can be the thing that was missing:
//
//   - the mail arrives (activity.captured), the ordinary case;
//   - the PERSON arrives (person.created). A sender nobody had classified yet
//     has no contact while their mail lands, so the capture door queues a pass
//     that cannot see them. The counterparty verdict mints them ten minutes
//     later and that mail becomes readable at exactly that moment — which is
//     the FIRST mail from a new correspondent, the one most likely to carry a
//     full signature block. Without this door nothing queues a pass, and the
//     24-hour reconciler is what eventually reads them;
//   - the mail OPENS (activity.updated naming a workspace audience). A message
//     held while its thread awaited a confidentiality answer is not signature
//     material — the pass reads open mail only — and the verdict that opens it
//     writes that event through the derivation.
//
// All three queue the same deduplicated pass, so a verdict that both mints a
// person and opens their mail costs one job rather than two.

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// captureEnrichFreshWindow is how old a capture event may be and still queue a
// pass.
//
// This consumer exists for PROMPTNESS alone — the nightly pass already owns
// everything older — and the bound is what makes first deployment safe: a new
// consumer group starts at stream position 0, so the first boot replays the
// whole activity stream into this handler and without the bound every
// historical message would mint a job. An hour is generous for a live event's
// delivery lag while excluding any replayed backlog.
const captureEnrichFreshWindow = time.Hour

// CaptureEnrichTrigger queues one signature-enrich pass per captured email.
type CaptureEnrichTrigger struct {
	pool    *pgxpool.Pool
	enqueue *jobs.Runner
	log     *slog.Logger
}

// NewCaptureEnrichTrigger builds the trigger over an insert-only jobs runner.
func NewCaptureEnrichTrigger(pool *pgxpool.Pool, enqueue *jobs.Runner, log *slog.Logger) *CaptureEnrichTrigger {
	return &CaptureEnrichTrigger{pool: pool, enqueue: enqueue, log: log}
}

// queues answers whether this envelope can have made somebody newly readable.
//
// An unreadable payload answers no rather than guessing: the reconciler covers
// whatever the event announced, and a consumer that wedged its group on one
// malformed body would stop reading every later one.
func (g *CaptureEnrichTrigger) queues(ctx context.Context, env events.Envelope) bool {
	switch env.Type {
	case eventActivityCaptured:
		// EMAIL only, decided before anything is queued. The signature pass
		// reads a mail's trailing lines and nothing else can carry a signature
		// block, so a meeting or a call would queue a model-backed pass that has
		// no work to do — this consumer's whole job is promptness, and paying
		// for a pass per logged call is not that.
		var payload crmcontracts.PublicEventActivityCaptured
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			g.log.WarnContext(ctx, "capture enrich trigger: unreadable capture payload",
				"event", env.EventID.String(), "err", err)
			return false
		}
		// The generated enum, not a literal: crm.yaml owns this vocabulary, and
		// a word hand-typed here would not move when the contract does. The
		// same comparison the vCard trigger makes on the same field.
		return payload.Kind == string(crmcontracts.ActivityKindEmail)
	case personCreatedEvent:
		// Every new contact, not only the ones a verdict minted. The event does
		// not say who created the person, and asking would be this consumer
		// guessing at the pass's own selection: a hand-typed contact with no
		// mail simply is not a candidate, which costs one query that returns
		// nobody. Narrowing here to the capture-created case would instead mean
		// two spellings of who is due, and the quieter one wins arguments.
		return true
	case eventActivityUpdated:
		// Only an OPENING. The pass reads workspace mail, so a message narrowed
		// to its participants is not new work — and the derivation emits this
		// event for a narrowing exactly as it does for a widening, so a
		// consumer that skipped the check would queue a model-backed pass every
		// time a message was held.
		var payload crmcontracts.PublicEventActivityUpdated
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			g.log.WarnContext(ctx, "capture enrich trigger: unreadable update payload",
				"event", env.EventID.String(), "err", err)
			return false
		}
		audience := payload.ChangedFields.Audience
		return audience != nil && *audience == crmcontracts.Workspace
	}
	return false
}

// HandleEvent routes one envelope. An event this consumer does not care about
// answers nil, so the group keeps flowing rather than wedging on somebody
// else's traffic. An enqueue failure comes back as an error and the bus
// redelivers — safe, because a redelivered event dedupes onto the pass the
// first delivery queued, and the nightly pass still reconciles what slips
// through.
func (g *CaptureEnrichTrigger) HandleEvent(ctx context.Context, env events.Envelope) error {
	if !g.queues(ctx, env) {
		return nil
	}
	// A stale event has no promptness left to buy: it is either replayed
	// backlog or a delivery the bus held for hours, and in both cases the
	// nightly pass already covers it. Skipping is nil, not an error — erroring
	// would redeliver the same stale event forever.
	//
	// Asked AFTER the type routing rather than before it, so the cheap answer
	// stays cheap: an event of a type this consumer ignores never reaches the
	// clock at all.
	if time.Since(env.OccurredAt) > captureEnrichFreshWindow {
		return nil
	}
	// The uniqueness is the flood bound: a mailbox sync lands hundreds of
	// messages in seconds, and without it each one would put its own row on
	// the ai_capture queue. ByArgs over the active states collapses the burst
	// onto the one pass already queued or running for this workspace.
	//
	// The states include running because River requires it in any custom list,
	// and that opens the one hole this trigger accepts: mail that arrives
	// mid-pass can dedupe against a pass that already listed its candidates,
	// and then waits for the nightly run. The alternative — no uniqueness —
	// trades that bounded promptness miss for an unbounded queue, which is
	// worse. Same states as the scheduled tick's own insert, so the two doors
	// dedupe against each other rather than stacking.
	//
	// The PASS, not a per-workspace child: the child kind went with the
	// fan-out (ADR-0103). Its args carried the workspace, so ByArgs collapsed a
	// burst onto the pass queued for THAT workspace; the pass carries none, so
	// the same dedupe now collapses onto the one pending pass — which is what a
	// flood bound wanted, and what it already meant on an installation with a
	// single workspace.
	child := CaptureEnrichArgs{}
	opts := oneOffPassOpts(child.Kind())
	opts.UniqueOpts = river.UniqueOpts{ByArgs: true, ByState: activeSweepStates}
	return g.enqueue.Enqueue(ctx, child, opts)
}
