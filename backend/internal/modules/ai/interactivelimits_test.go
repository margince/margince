// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// cloudRung and otherRung are two cloud models; withLimits measures cloudRung
// and the slowWalk rungs.
var (
	cloudRung = boundModel{provider: "openai", model: "measured"}
	otherRung = boundModel{provider: "openai", model: "unmeasured"}
	walkRungs = []boundModel{{provider: "openai", model: string(TierCheapCloud)}, {provider: "openai", model: string(TierPremium)}}
)

// withLimits swaps the generated table for one cold_start row, so a test reads the rule
// rather than today's records. These tests never run in parallel.
func withLimits(t *testing.T, limit time.Duration, measured ...boundModel) {
	t.Helper()
	was := certifiedAttemptTimeouts
	certifiedAttemptTimeouts = map[Task]certifiedLimit{TaskColdStart: {limit: limit, measured: measured}}
	t.Cleanup(func() { certifiedAttemptTimeouts = was })
}

func TestTheInteractiveLimitIsHalfAgainTheP95WithinTheFloorAndTheCap(t *testing.T) {
	for p95, want := range map[time.Duration]time.Duration{
		4303 * time.Millisecond:   InteractiveAttemptFloor,
		59190 * time.Millisecond:  89 * time.Second,
		60 * time.Second:          90 * time.Second,
		223062 * time.Millisecond: CallCeiling,
	} {
		if got := InteractiveAttemptTimeout(p95); got != want {
			t.Errorf("a p95 of %s gives %s, want %s", p95, got, want)
		}
	}
}

func TestOnlyAWalkOfMeasuredCloudModelsGetsTheLimit(t *testing.T) {
	withLimits(t, time.Minute, cloudRung, boundModel{provider: providerOllama, model: "listed"})
	today := EffectiveTask{DecisionTimeout: DecisionCallTimeout, AttemptTimeout: CallCeiling}
	cases := map[string]struct {
		rungs []boundModel
		want  EffectiveTask
	}{
		"every rung measured":       {[]boundModel{cloudRung}, EffectiveTask{DecisionTimeout: DecisionCallTimeout, AttemptTimeout: time.Minute, WalkTimeout: 2 * time.Minute}},
		"a rung no record measured": {[]boundModel{cloudRung, otherRung}, today},
		"a local rung":              {[]boundModel{{provider: providerOllama, model: "listed"}}, today},
		"no rung yet":               {nil, today},
	}
	for name, c := range cases {
		if got := TaskDefaults(TaskColdStart, c.rungs); got != c.want {
			t.Errorf("%s: %+v, want %+v", name, got, c.want)
		}
	}
	withLimits(t, 200*time.Second, cloudRung)
	if got := TaskDefaults(TaskColdStart, []boundModel{cloudRung}).WalkTimeout; got != CallCeiling {
		t.Errorf("a 200 s limit bounds the walk at %s, want the %s cap", got, CallCeiling)
	}
}

func TestATaskNoRowCoversKeepsTodaysDefaults(t *testing.T) {
	withLimits(t, time.Minute, cloudRung)
	want := EffectiveTask{AttemptTimeout: CallCeiling, DecisionTimeout: DecisionCallTimeout}
	for _, task := range []Task{TaskTranscript, TaskAccountScan} {
		if got := TaskDefaults(task, []boundModel{cloudRung}); got != want {
			t.Errorf("%s defaults to %+v, want %+v", task, got, want)
		}
	}
}

func TestTheAdminOverrideWinsAndTheWalkBoundFollowsItUncapped(t *testing.T) {
	withLimits(t, time.Minute, cloudRung)
	measured := []boundModel{cloudRung}
	cases := map[string]struct {
		override TaskOverrides
		task     Task
		rungs    []boundModel
		want     EffectiveTask
	}{
		"lowered": {
			TaskOverrides{TaskColdStart: {AttemptTimeoutMs: 20000}},
			TaskColdStart, measured,
			EffectiveTask{DecisionTimeout: DecisionCallTimeout, AttemptTimeout: 20 * time.Second, WalkTimeout: 40 * time.Second},
		},
		"raised past half the cap": {
			TaskOverrides{TaskColdStart: {AttemptTimeoutMs: 200000}},
			TaskColdStart, measured,
			EffectiveTask{DecisionTimeout: DecisionCallTimeout, AttemptTimeout: 200 * time.Second, WalkTimeout: 400 * time.Second},
		},
		"on an unmeasured walk": {
			TaskOverrides{TaskColdStart: {AttemptTimeoutMs: 20000}},
			TaskColdStart,
			[]boundModel{otherRung},
			EffectiveTask{DecisionTimeout: DecisionCallTimeout, AttemptTimeout: 20 * time.Second},
		},
		"on a background task": {
			TaskOverrides{TaskAccountScan: {AttemptTimeoutMs: 20000}},
			TaskAccountScan, measured,
			EffectiveTask{DecisionTimeout: DecisionCallTimeout, AttemptTimeout: 20 * time.Second},
		},
	}
	for name, c := range cases {
		if got := c.override.Effective(c.task, c.rungs); got != c.want {
			t.Errorf("%s: %+v, want %+v", name, got, c.want)
		}
	}
}

// fakeClock is the router's clock in these tests; a slowClient moves it by the
// time each call takes.
type fakeClock struct{ at time.Time }

// tryTimeoutKey carries the timeout the router gave a try, so a client reads it
// without a real clock.
type tryTimeoutKey struct{}

// slowClient takes `takes` on the fake clock, or its whole timeout when that is
// shorter, which it then reports as a deadline.
type slowClient struct {
	stubClient
	clock *fakeClock
	takes time.Duration
	text  string
	calls *int
}

func (s slowClient) Complete(ctx context.Context, _ model.Request) (model.Response, error) {
	*s.calls++
	limit, ok := ctx.Value(tryTimeoutKey{}).(time.Duration)
	if !ok {
		return model.Response{}, errors.New("the router gave this try no timeout")
	}
	if s.takes > limit {
		s.clock.at = s.clock.at.Add(limit)
		return model.Response{}, fmt.Errorf("provider: %w", context.DeadlineExceeded)
	}
	s.clock.at = s.clock.at.Add(s.takes)
	return model.Response{Text: s.text, InputTokens: 1, OutputTokens: 1}, nil
}

// walkResult is what one slowWalk saw: the tries, the time the user waited,
// the error and the trace.
type walkResult struct {
	tries  int
	waited time.Duration
	err    error
	rows   []Call
}

// slowWalk drives one structured call of task whose every answer the validator
// refuses, on a router whose rungs reach walkRungs.
func slowWalk(t *testing.T, task Task, clients map[Tier]slowClient) walkResult {
	t.Helper()
	clock := &fakeClock{at: time.Unix(0, 0)}
	calls := 0
	bound, meta := map[Tier]model.Client{}, map[Tier]routeMeta{}
	for tier, client := range clients {
		client.clock, client.calls = clock, &calls
		bound[tier], meta[tier] = client, routeMeta{provider: "openai", model: string(tier)}
	}
	fcs := &fakeCallStore{}
	r := assembleRouter(bound, stubClient{}, ProfileCloudFrontier, stubMeter{}, unlimitedBudget{}, fcs, meta, false, nil)
	r.now = func() time.Time { return clock.at }
	r.tryDeadline = func(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
		return context.WithCancel(context.WithValue(ctx, tryTimeoutKey{}, d))
	}
	r.cacheOff = true
	_, _, err := r.CompleteStructured(wsCtx(), task, model.Request{}, func(string) error { return errors.New("refused") })
	if err == nil {
		t.Fatalf("%s: a walk the validator never accepts returned no error", task)
	}
	if len(fcs.recorded) != calls {
		t.Fatalf("%s: %d tries left %d rows, want one row for each try", task, calls, len(fcs.recorded))
	}
	return walkResult{tries: calls, waited: clock.at.Sub(time.Unix(0, 0)), err: err, rows: fcs.recorded}
}

// The walk that makes the most tries: cheap_cloud never answers, and premium
// answers in 30 s with text the validator refuses.
var hungThenRefused = map[Tier]slowClient{
	TierCheapCloud: {takes: time.Hour},
	TierPremium:    {takes: 30 * time.Second, text: "not the schema"},
}

func TestTheWalkBoundStopsASlowInteractiveCallAsAnUnansweredWalk(t *testing.T) {
	withLimits(t, 89*time.Second, walkRungs...)
	got := slowWalk(t, TaskColdStart, hungThenRefused)
	if got.tries != 3 || got.waited != 178*time.Second {
		t.Fatalf("cold_start made %d tries over %s, want 3 over its 2m58s walk bound", got.tries, got.waited)
	}
	if !errors.Is(got.err, ErrAllTiersFailed) || !isDeadline(got.err) {
		t.Fatalf("the spent walk ended on %v, want ErrAllTiersFailed that reads as a deadline", got.err)
	}
	if sentinel := got.rows[len(got.rows)-1].ErrorSentinel; sentinel != sentinelTimeout {
		t.Fatalf("the rung that spent the bound ends on %q, want %q", sentinel, sentinelTimeout)
	}
}

// A refused answer that lands as the bound runs out leaves no further walk, and
// the call ends as the refusal: the model did answer.
func TestARefusedAnswerThatSpendsTheBoundEndsAsARefusal(t *testing.T) {
	withLimits(t, time.Minute, walkRungs...)
	refused := slowClient{takes: time.Minute, text: "not the schema"}
	for name, clients := range map[string]map[Tier]slowClient{
		"after the first walk": {TierCheapCloud: {takes: time.Hour}, TierPremium: refused},
		"after the retry":      {TierPremium: refused},
	} {
		got := slowWalk(t, TaskColdStart, clients)
		if got.tries != 2 || got.waited != 2*time.Minute {
			t.Errorf("%s: %d tries over %s, want 2 over the 2m0s walk bound", name, got.tries, got.waited)
		}
		if !errors.Is(got.err, ErrOutputRejected) || isDeadline(got.err) {
			t.Errorf("%s: the call ended on %v, want the refusal", name, got.err)
		}
		if sentinel := got.rows[len(got.rows)-1].ErrorSentinel; sentinel != sentinelOutputRejected {
			t.Errorf("%s: the answered try ends on %q, want %q", name, sentinel, sentinelOutputRejected)
		}
	}
}

// A try needs at least the floor; with less left of the bound none starts.
func TestNoTryStartsOnLessThanTheFloor(t *testing.T) {
	withLimits(t, time.Minute, walkRungs...)
	clients := map[Tier]slowClient{
		TierCheapCloud: {takes: time.Hour},
		TierPremium:    {takes: 35 * time.Second, text: "not the schema"},
	}
	got := slowWalk(t, TaskColdStart, clients)
	if got.tries != 2 || got.waited != 95*time.Second {
		t.Fatalf("%d tries over %s, want 2 over 1m35s: 25 s was left, under the %s floor", got.tries, got.waited, InteractiveAttemptFloor)
	}
}

// Five tries at today's limits: the walk a background task, a task no row
// covers, and a walk with an unmeasured rung all still make.
func TestAnUnboundedWalkMakesEveryTry(t *testing.T) {
	withLimits(t, time.Minute, walkRungs[1])
	for _, task := range []Task{TaskAccountScan, TaskTranscript, TaskColdStart} {
		got := slowWalk(t, task, hungThenRefused)
		if want := 2*CallCeiling + 3*30*time.Second; got.tries != 5 || got.waited != want {
			t.Errorf("%s made %d tries over %s, want 5 over %s", task, got.tries, got.waited, want)
		}
	}
}

// The bound counts from the first ladder try, so time before it spends none.
func TestTheWalkBoundStartsAtTheFirstTry(t *testing.T) {
	withLimits(t, time.Minute, walkRungs...)
	clock := &fakeClock{at: time.Unix(0, 0)}
	r := newTracingRouter(t, stubClient{}, &fakeCallStore{})
	r.now = func() time.Time { return clock.at }
	lc := newLogicalCall()
	r.callSettings(lc, TaskColdStart)
	clock.at = clock.at.Add(time.Hour)
	r.startWalk(lc, TaskColdStart, &binding{routeMeta: map[Tier]routeMeta{TierCheapCloud: {provider: "openai", model: string(TierCheapCloud)}}}, []Tier{TierCheapCloud})
	if got, err := r.tryTimeout(lc, TaskColdStart); err != nil || got != time.Minute {
		t.Fatalf("the first try after an hour of preparation gets %s, %v; want the whole minute", got, err)
	}
}

// The call page reads each try's limit and the walk bound off the snapshot.
func TestTheSnapshotRecordsTheTryLimitAndTheWalkBound(t *testing.T) {
	// twoRungRouter's cheap_cloud rung reaches openai/gpt-x.
	withLimits(t, time.Minute, boundModel{provider: "openai", model: "gpt-x"})
	fcs := &fakeCallStore{}
	r := twoRungRouter(t, stubClient{}, stubClient{resp: model.Response{Text: "ok", InputTokens: 1, OutputTokens: 1}}, fcs)
	lane := ProviderConfig{Provider: providerOpenAICompatible, BaseURL: "https://openrouter.ai/api", Model: "m"}
	r.install(r.binding().withConfig(RoutingConfig{Tiers: map[Tier]ProviderConfig{TierCheapCloud: lane}}, nil))
	if _, _, err := r.serveCompletion(wsCtx(), TaskColdStart, []Tier{TierCheapCloud}, model.Request{}); err != nil {
		t.Fatal(err)
	}
	if len(fcs.configSnapshots) != 1 {
		t.Fatalf("%d snapshots, want one", len(fcs.configSnapshots))
	}
	var params struct {
		DeadlineMs  int64 `json:"deadline_ms"`
		WalkBoundMs int64 `json:"walk_bound_ms"`
	}
	if err := json.Unmarshal(fcs.configSnapshots[0].ProviderParams, &params); err != nil {
		t.Fatal(err)
	}
	if params.DeadlineMs != 60000 || params.WalkBoundMs != 120000 {
		t.Fatalf("recorded deadline_ms %d and walk_bound_ms %d, want 60000 and 120000", params.DeadlineMs, params.WalkBoundMs)
	}
}
