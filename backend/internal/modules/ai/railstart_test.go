// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// countingStarter records how often the rail was told a call began and how
// often its lease was renewed, and still records the calls themselves — so one
// recorder can drive a real router call and answer every half of what that call
// owes the rail.
type countingStarter struct {
	fakeCallStore
	starts   []Call
	leases   []time.Duration
	renewals []renewal
	// claim is what every start answers; the assertions on a renewal are that
	// it repeats exactly this.
	claim RailClaim
	// refuse makes every start answer false, the way a start that could not be
	// written does.
	refuse bool
}

// renewal is one call to RenewRailLease, as the router made it.
type renewal struct {
	call  Call
	claim RailClaim
	lease time.Duration
}

func (c *countingStarter) AnnounceRailStart(_ context.Context, call Call, lease time.Duration) (RailClaim, bool) {
	c.starts = append(c.starts, call)
	c.leases = append(c.leases, lease)
	if c.refuse {
		return RailClaim{}, false
	}
	return c.claim, true
}

func (c *countingStarter) RenewRailLease(_ context.Context, call Call, claim RailClaim, lease time.Duration) {
	c.renewals = append(c.renewals, renewal{call: call, claim: claim, lease: lease})
}

// aClaim is a distinctive claim, so a renewal that rebuilt one of its own would
// be told apart from one that repeated the start's.
func aClaim() RailClaim {
	return RailClaim{Attempt: 4, StartedAt: time.Date(2026, 3, 9, 8, 15, 0, 0, time.UTC)}
}

// railRouter serves cheap_cloud from client, with starter as the recorder, so
// every test below drives the production path rather than the announcement by
// hand.
func railRouter(client model.Client, starter *countingStarter) *Router {
	return assembleRouter(
		map[Tier]model.Client{TierCheapCloud: client},
		nil, ProfileCloudFrontier, stubMeter{}, unlimitedBudget{}, starter,
		map[Tier]routeMeta{TierCheapCloud: {provider: "openai", model: "gpt-cheap"}},
		false, nil,
	)
}

// One lease covers one model call and the flush that may settle it — and no
// more, because the lease is renewed before every further call. Stated as
// inequalities against the two bounds it must outlast rather than as the
// formula, so the test fails for a constant that happens to be large enough
// today and for one that forgot the flush.
//
// The upper bound is the point of the whole shape: a lease is how long a dead
// process is believed, and one sized for more than the call it precedes is
// believing a dead process for work it never started.
func TestOneLeaseOutlastsOneModelCallAndTheFlush(t *testing.T) {
	got := railLease()
	if got <= CallCeiling {
		t.Errorf("railLease() = %s, which does not outlast the %s a single model call can spend — a healthy call would render stalled", got, CallCeiling)
	}
	if got-CallCeiling < traceWriteTimeout {
		t.Errorf("railLease() leaves %s past the call for the flush, which is allowed %s — a settle that lands late finds the row already stalled", got-CallCeiling, traceWriteTimeout)
	}
	if got >= 2*CallCeiling {
		t.Errorf("railLease() = %s covers more than one model call, so a process killed mid-call is believed for calls it never made", got)
	}
}

// CompleteStructured threads ONE logical call through up to three attempts —
// the first try, the schema-invalid retry, the tier escalation. They are rungs
// of one thing a reader asked for once, so the rail is told once.
//
// The failure this prevents is not cosmetic: a second start carries a higher
// attempt, which outranks the first attempt's settle, so the occurrence would
// reopen and one request would report as several starts.
func TestOneLogicalCallAnnouncesItsStartOnce(t *testing.T) {
	starter := &countingStarter{claim: aClaim()}
	r := assembleRouter(nil, nil, ProfileEUHosted, &memoryMeter{}, StaticBudget(0), starter, nil, false, nil)
	lc := newLogicalCall()
	ctx := principal.WithCorrelationID(context.Background(), ids.NewV7())

	for range 3 {
		lc.announceRailStartOnce(ctx, r, TaskSummarize)
	}

	if len(starter.starts) != 1 {
		t.Fatalf("three attempts of one logical call announced %d starts, want 1", len(starter.starts))
	}
	// The lease is asserted here and nowhere else. Without it the recorder holds
	// a field no test reads, and the claim this whole file rests on — that the
	// starter is handed the DERIVED lease — would survive that argument being
	// dropped or replaced by a constant.
	if want := railLease(); starter.leases[0] != want {
		t.Errorf("the start was leased for %s, want the derived %s", starter.leases[0], want)
	}
}

// A recorder that cannot reach Postgres is not asked to pretend. The DB-less
// local router and the cert lane both inject one, and the honest behaviour is
// no announcement rather than a no-op method they were forced to grow.
func TestARecorderThatCannotAnnounceIsNotAskedTo(t *testing.T) {
	starter := &countingStarter{}
	r := assembleRouter(nil, nil, ProfileEUHosted, &memoryMeter{}, StaticBudget(0), &memCallStore{}, nil, false, nil)
	lc := newLogicalCall()
	ctx := principal.WithCorrelationID(context.Background(), ids.NewV7())

	// The assertion is that this does not panic and marks nothing: a type
	// assertion that succeeded against a recorder with no database would fail
	// inside AnnounceRailStart instead, one layer further from the cause.
	lc.announceRailStartOnce(ctx, r, TaskSummarize)

	if lc.railAnnounced {
		t.Error("a recorder that announces nothing marked the call announced, so a recorder that CAN announce would be skipped after it")
	}
	if len(starter.leases) != 0 {
		t.Errorf("a recorder with no database was handed %d lease(s)", len(starter.leases))
	}
}

// The PRODUCTION call site, which nothing else here exercises.
//
// Every other test in this file drives announceRailStartOnce by hand, and the
// integration tests drive CallMeter.AnnounceRailStart by hand. Both would pass
// against a router that never announces anything — which is exactly the
// behaviour this feature replaces. So this one serves a real call through
// serveCompletion and asserts the rail heard about it, with the identity it
// needs to pair the start with the settle that follows.
func TestServingACallAnnouncesItsStartToTheRail(t *testing.T) {
	starter := &countingStarter{claim: aClaim()}
	r := railRouter(stubClient{resp: model.Response{Text: "answer"}}, starter)
	corr := ids.NewV7()
	ctx := principal.WithCorrelationID(wsCtx(), corr)

	if _, _, err := r.serveCompletion(ctx, TaskSummarize, []Tier{TierCheapCloud}, model.Request{}); err != nil {
		t.Fatalf("serving the call: %v", err)
	}

	if len(starter.starts) != 1 {
		t.Fatalf("serving one call announced %d starts, want 1", len(starter.starts))
	}
	got := starter.starts[0]
	if got.Task != TaskSummarize {
		t.Errorf("the start named task %q, want %q", got.Task, TaskSummarize)
	}
	// The correlation id is what pairs this start with its own settle. A start
	// carrying a different one opens an occurrence the flush never closes.
	if got.CorrelationID == nil || *got.CorrelationID != corr {
		t.Errorf("the start carried correlation %v, want %s — the settle would land on a different occurrence", got.CorrelationID, corr)
	}
}

// The start is made for the model call that follows it, so that call spends the
// lease rather than renewing it. A renewal here would be a second write per
// call for nothing, and — more to the point — would make the count of renewals
// meaningless as the count of calls the lease was NOT already sized for.
func TestTheFirstModelCallSpendsTheLeaseTheStartMade(t *testing.T) {
	starter := &countingStarter{claim: aClaim()}
	r := railRouter(stubClient{resp: model.Response{Text: "answer"}}, starter)
	ctx := principal.WithCorrelationID(wsCtx(), ids.NewV7())

	if _, _, err := r.serveCompletion(ctx, TaskSummarize, []Tier{TierCheapCloud}, model.Request{}); err != nil {
		t.Fatalf("serving the call: %v", err)
	}

	if len(starter.renewals) != 0 {
		t.Errorf("a single-call serve renewed its lease %d time(s), want 0 — the start already covers that call", len(starter.renewals))
	}
}

// Every model call after the first renews the lease, with the claim the start
// made. This is what lets one lease be sized for one call: a rung the walk
// moves past has spent the lease in force, and the next rung must not run on
// whatever is left of it.
//
// Two bound rungs, the first failing, so the walk really makes two calls. The
// renewal is asserted to REPEAT the start's claim: a renewal that rebuilt the
// claim would move started_at or land on a different attempt, and the
// projection would take either as a new fact about the row rather than a
// longer lease on the same one.
func TestEveryModelCallAfterTheFirstRenewsTheLease(t *testing.T) {
	starter := &countingStarter{claim: aClaim()}
	r := assembleRouter(
		map[Tier]model.Client{
			TierCheapCloud: stubClient{err: errors.New("cheap rung down")},
			TierPremium:    stubClient{resp: model.Response{Text: "answer"}},
		},
		nil, ProfileCloudFrontier, stubMeter{}, unlimitedBudget{}, starter,
		map[Tier]routeMeta{
			TierCheapCloud: {provider: "openai", model: "gpt-cheap"},
			TierPremium:    {provider: "openai", model: "gpt-premium"},
		},
		false, nil,
	)
	ctx := principal.WithCorrelationID(wsCtx(), ids.NewV7())

	if _, _, err := r.serveCompletion(ctx, TaskSummarize, []Tier{TierCheapCloud, TierPremium}, model.Request{}); err != nil {
		t.Fatalf("serving the call: %v", err)
	}

	if len(starter.renewals) != 1 {
		t.Fatalf("a walk over two rungs renewed the lease %d time(s), want 1 — one per model call after the first", len(starter.renewals))
	}
	got := starter.renewals[0]
	if got.claim != aClaim() {
		t.Errorf("the renewal carried claim %+v, want the start's %+v — anything else rewrites the row instead of extending it", got.claim, aClaim())
	}
	if got.lease != railLease() {
		t.Errorf("the renewal was leased for %s, want the derived %s", got.lease, railLease())
	}
	if want := starter.starts[0]; got.call.Task != want.Task || got.call.LogicalCallID != want.LogicalCallID {
		t.Errorf("the renewal named %s/%s, and the start named %s/%s — a renewal under another identity opens a second occurrence", got.call.Task, got.call.LogicalCallID, want.Task, want.LogicalCallID)
	}
}

// A structured call is ONE piece of work however many walks it takes, and the
// production path is what has to know that. Driven through CompleteStructured
// with a validator that never accepts, so all maxLadderWalks walks really run.
func TestAStructuredCallAnnouncesOneStartAcrossEveryWalk(t *testing.T) {
	starter := &countingStarter{claim: aClaim()}
	r := railRouter(stubClient{resp: model.Response{Text: "answer"}}, starter)
	ctx := principal.WithCorrelationID(wsCtx(), ids.NewV7())
	rejectEverything := func(string) error { return errors.New("never valid") }

	if _, _, err := r.CompleteStructured(ctx, TaskSummarize, model.Request{}, rejectEverything); err == nil {
		t.Fatal("a validator that never accepts returned no error, so the retry and escalation did not run")
	}

	if len(starter.starts) != 1 {
		t.Fatalf("a structured call that walked the ladder every time announced %d starts, want 1 — "+
			"a second start outranks the first attempt's settle and reports one request as several", len(starter.starts))
	}
}

// A structured call's later walks are the model calls the start could not be
// sized for, so each one renews. This is the case the lease used to be
// inflated for: with one rung bound, three walks are three calls, and the
// occurrence has to stay believable through all of them on leases sized for
// one.
func TestAStructuredCallRenewsBeforeEveryWalkAfterTheFirst(t *testing.T) {
	starter := &countingStarter{claim: aClaim()}
	r := railRouter(stubClient{resp: model.Response{Text: "answer"}}, starter)
	ctx := principal.WithCorrelationID(wsCtx(), ids.NewV7())

	if _, _, err := r.CompleteStructured(ctx, TaskSummarize, model.Request{},
		func(string) error { return errors.New("never valid") }); err == nil {
		t.Fatal("a validator that never accepts returned no error")
	}

	calls := len(starter.recorded)
	if got := len(starter.renewals); got != calls-1 {
		t.Errorf("%d model calls renewed the lease %d time(s), want %d — every call after the first", calls, got, calls-1)
	}
	for i, got := range starter.renewals {
		if got.claim != aClaim() {
			t.Errorf("renewal %d carried claim %+v, want the start's %+v", i+1, got.claim, aClaim())
		}
	}
}

// A start that was not written has no row to keep alive, so a renewal for it
// would be a start under another name: the projection would take the first
// renewal as the opening fact and the flush's settle would then close a row
// the start never opened. Nothing is renewed until something was started.
func TestAStartThatWasNotMadeIsNotRenewed(t *testing.T) {
	starter := &countingStarter{refuse: true}
	r := assembleRouter(
		map[Tier]model.Client{
			TierCheapCloud: stubClient{err: errors.New("cheap rung down")},
			TierPremium:    stubClient{resp: model.Response{Text: "answer"}},
		},
		nil, ProfileCloudFrontier, stubMeter{}, unlimitedBudget{}, starter,
		map[Tier]routeMeta{
			TierCheapCloud: {provider: "openai", model: "gpt-cheap"},
			TierPremium:    {provider: "openai", model: "gpt-premium"},
		},
		false, nil,
	)
	ctx := principal.WithCorrelationID(wsCtx(), ids.NewV7())

	if _, _, err := r.serveCompletion(ctx, TaskSummarize, []Tier{TierCheapCloud, TierPremium}, model.Request{}); err != nil {
		t.Fatalf("serving the call: %v", err)
	}

	if len(starter.starts) != 1 {
		t.Fatalf("the start was attempted %d time(s), want 1", len(starter.starts))
	}
	if len(starter.renewals) != 0 {
		t.Errorf("a start that answered false was renewed %d time(s), want 0", len(starter.renewals))
	}
}

// RouteWriteDeadline's worst case is arithmetic over maxLadderWalks, and this
// is what keeps that number honest: a fourth walk added to CompleteStructured
// without changing the constant would cut a healthy call mid-response. Counted
// from the calls the router actually recorded, so the number is observed rather
// than restated.
func TestStructuredWalksTheLadderNoMoreThanTheRouteDeadlineAssumes(t *testing.T) {
	starter := &countingStarter{claim: aClaim()}
	r := railRouter(stubClient{resp: model.Response{Text: "answer"}}, starter)
	ctx := principal.WithCorrelationID(wsCtx(), ids.NewV7())

	if _, _, err := r.CompleteStructured(ctx, TaskSummarize, model.Request{},
		func(string) error { return errors.New("never valid") }); err == nil {
		t.Fatal("a validator that never accepts returned no error")
	}

	// One rung, so each walk of the ladder is exactly one recorded attempt.
	if walks := len(starter.recorded); walks > maxLadderWalks {
		t.Errorf("CompleteStructured walked the ladder %d times and RouteWriteDeadline is sized for %d — "+
			"a healthy call now outlives the deadline that lets it answer", walks, maxLadderWalks)
	}
}
