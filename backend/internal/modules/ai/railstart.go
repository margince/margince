// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The router's OPENING announcement — the occurrence is live, before the model
// is asked anything — and the RENEWALS that keep it believable while the model
// is asked again.
//
// railemit.go announces the other end — what a call turned out to be, once it
// was over. That was the whole rail for router-owned work, and it is why a rep
// who asked for a summary saw nothing at all and then saw "done": the only
// moment the router spoke was after the moment worth watching had passed.
//
// The unit framed here is ONE LOGICAL CALL, not one unit of work, and the
// difference is the reason this file is small. A logical call has a beginning
// the router can observe (it is about to serve it) and an end it already
// observes (the flush), so the pair needs no scope, no teardown at whatever
// mints a correlation id, and no attempt that must survive a process. For the
// tasks a person triggers and then waits on — summarize, draft_reply,
// offer_draft, all registered oneShot — the logical call IS the unit of work,
// and framing it is the whole feature.
//
// A logical call is one occurrence but not one model call: the ladder has
// rungs, and CompleteStructured walks it up to maxLadderWalks times. The
// occurrence is announced once, so the settle keeps its attempt, and the lease
// is renewed before every model call after the first, so no single lease has to
// be sized for every rung of every walk. That sizing is what a lease's length
// really is — the time a process that died mid-call, or a flush that timed out,
// goes on being displayed as working — so a lease covering the whole logical
// call would have a rep watching a dead draft for up to three quarters of an
// hour.
//
// What that costs, honestly: a task whose unit of work spans MANY logical calls
// under one correlation id (the deep read's page-parallel fact lane) reopens
// its occurrence once per call, because a later call's attempt outranks the
// previous settle. The row still ends settled and nothing renders those kinds
// today, so the churn is real and invisible — but it is churn, and a unit-of-
// work frame is what would remove it rather than hide it.

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// railStateRunning is the one live state the router can honestly claim. It
// never says queued: by the time this file speaks, the call is being served.
const railStateRunning = "running"

// railStarter is the OPTIONAL half of CallRecorder.
//
// Optional rather than a third method on the interface, because CallRecorder
// has implementations with no database behind them at all — the cert lane's
// in-memory recorder and the DB-less local router seam (ai.WithCallStore).
// Widening the interface would force both to grow a method whose only honest
// body is a no-op, which is a worse lie than not implementing it: a recorder
// that cannot reach Postgres cannot announce, and saying so by NOT satisfying
// this interface is the accurate statement.
type railStarter interface {
	// AnnounceRailStart opens the occurrence as running and answers the claim
	// it made, or false when it made none.
	AnnounceRailStart(ctx context.Context, c Call, lease time.Duration) (RailClaim, bool)
	// RenewRailLease keeps the claim believable for lease beyond now.
	RenewRailLease(ctx context.Context, c Call, claim RailClaim, lease time.Duration)
}

// The real recorder is a starter, held at compile time: the router reaches the
// rail through a type assertion that a recorder is free to fail, and a CallMeter
// that stopped satisfying this interface would fail it quietly — every router
// call still traced, and none of them announced.
var _ railStarter = (*CallMeter)(nil)

// RailClaim is what the opening announcement said about the occurrence, and
// what every renewal has to say again: the attempt the settle will count to,
// and the instant that attempt became current.
//
// Carried rather than recomputed because the projection writes every column
// from the event it is handed. A renewal that recounted the attempt could land
// on a different one — a page-parallel sibling settling in between lifts the
// count — and reopen the row instead of extending it; one that read the clock
// afresh would move started_at, and the lease is aged from there. Both instants
// are the DATABASE's, which is the clock stale_after is compared against.
type RailClaim struct {
	Attempt   int
	StartedAt time.Time
}

// railLease is how long ONE announcement of a live router occurrence stays
// believable: one model call, plus the flush that may settle it.
//
// DERIVED from the bound it must outlast, never chosen. CallCeiling caps a
// SINGLE model call — the http.Client timeout every adapter is built with — and
// traceWriteTimeout bounds the flush, which is what settles the occurrence
// after the last call. Nothing between two calls of one logical call takes
// longer than either: the meter, the cache, the validator and the escalation
// are all in-process.
//
// One call and not the whole logical call, and the difference is what a lease
// IS: the time a process that died mid-call, or a flush that timed out, goes on
// being displayed as working. Every rung of every walk CompleteStructured can
// make adds up to three quarters of an hour, and a lease sized to cover the
// longest healthy call is by construction how long a dead one is believed. So
// the lease is kept to one call and renewed before each further one
// (logicalCall.renewRailLease), which is what the projection's renewal branch
// exists for.
//
// A round number would be a guess that happens to look like a decision, and the
// failure it buys is silent in both directions: too short renders healthy work
// as a dead worker, too long leaves a killed process claiming to work. Neither
// is visible in a test that does not run for minutes.
func railLease() time.Duration {
	return CallCeiling + traceWriteTimeout
}

// railOccurrence is the live occurrence one logical call holds open on the
// rail, and what keeps it believable: the starter that opened it, the call that
// identifies it, and the claim every renewal repeats.
type railOccurrence struct {
	starter railStarter
	call    Call
	claim   RailClaim
	// unspent is true while the lease in force has not yet been spent on a
	// model call. An announcement is made for the call that follows it, so that
	// call needs no renewal; every call after it does.
	unspent bool
}

// announceRailStartOnce opens this logical call's occurrence, at most once.
//
// ONCE is the whole reason this hangs off logicalCall rather than off Router.
// CompleteStructured threads one logicalCall through up to three serveAttempt
// calls — the first try, the schema-invalid retry, the tier escalation — and
// they are rungs of one piece of work a reader asked for once. Announcing per
// attempt would reopen the occurrence under a rising attempt twice, so the rail
// would report one request as three starts.
//
// It reads the correlation id off the SAME context value Call.CorrelationID is
// read from at flush, so the opening and closing announcements agree about
// which occurrence they describe, or neither is made.
//
// WHERE IT IS CALLED FROM is as load-bearing as what it does. serveAttempt
// calls it after the workspace check, the budget, the cache key and the profile
// — every return above those is untraced, so announcing higher would open an
// occurrence with no terminal trace behind it at all. From that point the
// deferred finalizeAttempt is armed, so the attempt APPENDS a terminal trace
// whatever it does next.
//
// Appending is not writing, and that gap is this placement's honest limit: the
// flush is best-effort by design (it must never fail a working model call), so
// a flush that times out — or a process that dies between the two — leaves a
// start nothing settles. The lease bounds how long such a row claims to be
// working, and aiactivity's sweep of abandoned router occurrences is what
// finally closes it. The settle is LIKELY, not guaranteed; a comment claiming
// otherwise would be the kind nobody re-checks.
func (lc *logicalCall) announceRailStartOnce(ctx context.Context, r *Router, task Task) {
	if lc.railAnnounced {
		return
	}
	starter, ok := r.calls.(railStarter)
	if !ok {
		return
	}
	// Set before the announcement rather than after it. A failed announce is
	// deliberately not retried on the next rung: the retry would be a SECOND
	// start for the same work, and a rail that is missing one line is a smaller
	// wrong than a rail that invents one.
	lc.railAnnounced = true
	c := Call{Task: task, LogicalCallID: lc.id}
	if cid, ok := principal.CorrelationID(ctx); ok {
		c.CorrelationID = &cid
	}
	// The subject rides the same context value the settle reads at
	// newAttemptTrace, so the two announcements name one record or neither.
	if subject, ok := SubjectOf(ctx); ok {
		c.Subject = subject
	}
	claim, announced := starter.AnnounceRailStart(ctx, c, railLease())
	if !announced {
		// Nothing to renew: a start that was not made has no row, and a
		// renewal would be a start under another name.
		return
	}
	lc.rail = &railOccurrence{starter: starter, call: c, claim: claim, unspent: true}
}

// renewRailLease keeps the occurrence believable across the model call about
// to be made, so that no lease has to cover more than one.
//
// Called before EVERY rung the ladder walk tries, on every walk. The first call
// after the announcement is the one the announcement was made for, so it spends
// that lease rather than renewing it; each call after it renews. Best-effort
// like the start, and for the same reason: the starter logs a renewal it could
// not make, and the lease already in force keeps its own bound, so the worst a
// failed renewal costs is a healthy call reading stalled for the rest of that
// rung.
func (lc *logicalCall) renewRailLease(ctx context.Context) {
	if lc.rail == nil {
		return
	}
	if lc.rail.unspent {
		lc.rail.unspent = false
		return
	}
	// A cancelled call is about to fail its next rung on the spot, and the
	// flush that follows settles the occurrence; renewing it first would only
	// log a renewal the cancelled context refuses to write.
	if ctx.Err() != nil {
		return
	}
	lc.rail.starter.RenewRailLease(ctx, lc.rail.call, lc.rail.claim, railLease())
}

// AnnounceRailStart publishes the occurrence as running, and never fails the
// call it is about to describe.
//
// It opens its OWN transaction rather than riding one, because there is no
// transaction to ride: the trace's transaction does not exist until the flush,
// which is the very thing that made the router settled-only. That is the one
// structural cost of speaking early, and it is why every failure below is a log
// line — a model call must not break because the rail could not say it started.
func (m *CallMeter) AnnounceRailStart(ctx context.Context, c Call, lease time.Duration) (RailClaim, bool) {
	if !RouterReports(c.Task) {
		return RailClaim{}, false
	}
	// Same refusal as the settling half, for the same reason: storekit.Emit
	// rejects an envelope with no correlation id, so a call outside a
	// correlation scope cannot produce an occurrence however the key is built.
	// Announcing the start of one the flush will never close would leave a row
	// claiming to work until its lease expired.
	if !announceable(c) {
		return RailClaim{}, false
	}
	var claim RailClaim
	err := m.db.Tx(ctx, func(tx pgx.Tx) error {
		var txErr error
		claim, txErr = m.announceRailStartTx(ctx, tx, c, lease)
		return txErr
	})
	if err != nil {
		m.log.ErrorContext(ctx, "ai: announcing the start of a model call to the AI-activity rail failed — the call runs and is traced, but shows on the rail only once it settles", "task", string(c.Task), "err", err)
		return RailClaim{}, false
	}
	return claim, true
}

// RenewRailLease publishes the claim again, believable for lease beyond the
// database's own now, and never fails the call it keeps alive.
//
// Its own transaction, like the start and for the same reason: the model call
// it precedes has no transaction to ride. The lease it sends is measured from
// the claim's start instant, because that is the instant the projection ages a
// running attempt from — and the start instant is repeated rather than re-read
// so the renewal extends the row instead of moving it.
func (m *CallMeter) RenewRailLease(ctx context.Context, c Call, claim RailClaim, lease time.Duration) {
	err := m.db.Tx(ctx, func(tx pgx.Tx) error {
		var now time.Time
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return fmt.Errorf("ai: reading the database clock for the rail renewal: %w", err)
		}
		return m.publishRailLive(ctx, tx, c, claim, now.Sub(claim.StartedAt)+lease)
	})
	if err != nil {
		m.log.ErrorContext(ctx, "ai: renewing a model call's lease on the AI-activity rail failed — the call runs and is traced, but reads stalled if it outlives the lease already in force", "task", string(c.Task), "err", err)
	}
}

// announceRailStartTx writes the ledger row and publishes the running state.
//
// No lock, unlike the settling half — and what that costs is worth stating
// exactly, because it is not nothing.
//
// The settle COUNTS terminal calls under a write-identity lock because two
// concurrent settles that computed one attempt would have an OUTCOME silently
// refused, and losing an outcome is losing a fact that nothing can recover.
//
// Two concurrent starts under one key lose something smaller and recoverable. B
// counts the same terminals as A, both publish `running` at that attempt, and
// the projection refuses the second as an equal (attempt, rank) event. The row
// reads running either way, so nothing is wrong on screen — but if A then
// SETTLES while B is still working, the row reads settled while B runs, until
// B's own settle reopens it at the next attempt. A live interval is suppressed;
// no fact is lost.
//
// That is accepted rather than overlooked, because it needs two logical calls
// of ONE task under ONE correlation id — which is the multi-call shape the file
// header already names as churn-prone, which no displayed kind has, and which a
// unit-of-work frame is what actually fixes. Paying a lock on every start to
// tighten a live interval nobody currently renders is the wrong trade.
func (m *CallMeter) announceRailStartTx(ctx context.Context, tx pgx.Tx, c Call, lease time.Duration) (RailClaim, error) {
	attempt, started, err := railStartAttempt(ctx, tx, c)
	if err != nil {
		return RailClaim{}, err
	}
	claim := RailClaim{Attempt: attempt, StartedAt: started}
	if err := m.publishRailLive(ctx, tx, c, claim, lease); err != nil {
		return RailClaim{}, err
	}
	return claim, nil
}

// publishRailLive writes the ledger row and publishes the claim as running,
// believable for lease beyond the claim's start instant.
//
// The start and every renewal go through here, so the two can only differ in
// the lease: the projection writes every column from the event it is handed,
// and a renewal that spelled the claim its own way would rewrite the row it
// meant to extend.
func (m *CallMeter) publishRailLive(ctx context.Context, tx pgx.Tx, c Call, claim RailClaim, lease time.Duration) error {
	key := unitOfWorkKey(c)
	// Rounded UP to whole seconds, never down, because a lease is a floor —
	// "believable for at least this long" — and the wire carries seconds. It
	// also keeps every renewal a strict extension: a renewal is measured from a
	// later clock reading than the start, and truncation would round the two
	// onto one instant whenever they fall within a second, which the projection
	// then refuses as a redelivery. At least one second on top of that, because
	// the projection reads a lease of 0 as NO lease, and a running occurrence
	// without one is exactly the row that claims to be working forever; a
	// caller passing a zero lease has asked for the shortest believable one,
	// not for an immortal row.
	seconds := max(int(math.Ceil(lease.Seconds())), 1)
	ledgerID, err := logRailStateChange(ctx, tx, key, railStateRunning)
	if err != nil {
		return err
	}
	task := string(c.Task)
	payload := crmcontracts.InternalEventAiTaskStateChanged{
		Source:        SourceRouter,
		OccurrenceKey: key,
		Kind:          task,
		AiTask:        &task,
		Attempt:       claim.Attempt,
		State:         railStateRunning,
		QueuedAt:      claim.StartedAt,
		// StartedAt is set and equal to QueuedAt because the router never
		// observes a queue: it announces the instant it begins serving. The
		// column is not decoration here — ai_task_run_queued_has_no_start
		// requires a non-queued state to carry one.
		StartedAt:    &claim.StartedAt,
		LeaseSeconds: &seconds,
	}
	c.Subject.stamp(&payload)
	if err := storekit.EmitPipelinePayload(ctx, tx, ledgerID, payload); err != nil {
		return fmt.Errorf("ai: publish rail start: %w", err)
	}
	return nil
}

// railStartAttempt is the attempt THIS call will settle under.
//
// One more than the terminal calls already recorded for the occurrence, because
// the settle counts the same rows once its own is written — so the two agree by
// construction for a logical call that is alone under its key. Every kind the
// rail DRAWS today is such a call (summarize, draft_reply and offer_draft are
// registered oneShot), which is not the same as every router-owned task: the
// registry also hands the router page-parallel work like site_fact_extract, and
// those are the calls the paragraph below is about.
//
// Where they can disagree is a page-PARALLEL fan-out under one correlation id:
// a sibling settling between this start and this settle lifts the settle's
// count above the start's. The occurrence then reopens at the higher attempt
// and settles there, which is the projection behaving correctly — a higher
// attempt outranks everything — and is why disagreement costs churn rather than
// a lost outcome.
//
// clock_timestamp(), not now(): now() is transaction-start, and this is a fresh
// transaction whose start is not the instant the call begins. Still the
// DATABASE's clock, because stale_after is derived from this value and compared
// against the database's now() at read time — a host clock would decide when the
// row reads stalled by the size of its own drift.
func railStartAttempt(ctx context.Context, tx pgx.Tx, c Call) (attempt int, started time.Time, err error) {
	row := tx.QueryRow(ctx, `
		SELECT count(*) + 1, clock_timestamp()
		  FROM ai_call
		 WHERE is_terminal
		   AND task = $1
		   AND correlation_id = $2::uuid`,
		string(c.Task), storekit.UUIDOrNil(*c.CorrelationID))
	if err := row.Scan(&attempt, &started); err != nil {
		return 0, time.Time{}, fmt.Errorf("ai: counting rail start attempts: %w", err)
	}
	return attempt, started, nil
}
