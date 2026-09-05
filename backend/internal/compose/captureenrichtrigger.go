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

// HandleEvent routes one envelope. An event this consumer does not care about
// answers nil, so the group keeps flowing rather than wedging on somebody
// else's traffic. An enqueue failure comes back as an error and the bus
// redelivers — safe, because a redelivered event dedupes onto the pass the
// first delivery queued, and the nightly pass still reconciles what slips
// through.
func (g *CaptureEnrichTrigger) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Type != "activity.captured" {
		return nil
	}
	// EMAIL only, decided before anything is queued. The signature pass reads
	// a mail's trailing lines and nothing else can carry a signature block, so
	// a meeting or a call would queue a model-backed pass that has no work to
	// do — this consumer's whole job is promptness, and paying for a pass per
	// logged call is not that.
	var payload crmcontracts.PublicEventActivityCaptured
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		// A payload this consumer cannot read is not a reason to wedge the
		// group or to guess: the nightly pass covers whatever it announced.
		g.log.WarnContext(ctx, "capture enrich trigger: unreadable capture payload",
			"event", env.EventID.String(), "err", err)
		return nil
	}
	if payload.Kind != "email" {
		return nil
	}
	// A stale event has no promptness left to buy: it is either replayed
	// backlog or a delivery the bus held for hours, and in both cases the
	// nightly pass already covers it. Skipping is nil, not an error — erroring
	// would redeliver the same stale event forever.
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
