// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

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

func TestATaskACertifiedLatencyMeasuresGetsItsLimitAndAWalkBound(t *testing.T) {
	limit, ok := certifiedAttemptTimeouts[TaskColdStart]
	if !ok || limit >= CallCeiling {
		t.Fatalf("cold_start's generated limit is %s (row %t), want one under the %s ceiling", limit, ok, CallCeiling)
	}
	got := TaskDefaults(TaskColdStart)
	if got.AttemptTimeout != limit || got.WalkTimeout != interactiveWalkTries*limit {
		t.Fatalf("cold_start defaults to %+v, want %s per try and %s for the walk", got, limit, interactiveWalkTries*limit)
	}
	if capped := TaskDefaults(TaskGrowthFit); capped.WalkTimeout != CallCeiling {
		t.Fatalf("growth_fit's walk is bounded at %s, want the %s cap", capped.WalkTimeout, CallCeiling)
	}
}

func TestAWaitedTaskNoRecordMeasuresKeepsTodaysDefaults(t *testing.T) {
	want := EffectiveTask{AttemptTimeout: CallCeiling, DecisionTimeout: DecisionCallTimeout}
	if got := TaskDefaults(TaskTranscript); got != want {
		t.Fatalf("transcript defaults to %+v, want %+v", got, want)
	}
}

func TestTheAdminOverrideWinsAndTheWalkBoundFollowsIt(t *testing.T) {
	v := TaskOverrides{
		TaskColdStart:   {AttemptTimeoutMs: 20000},
		TaskSummarize:   {AttemptTimeoutMs: 200000},
		TaskAccountScan: {AttemptTimeoutMs: 20000},
	}
	cases := map[Task]EffectiveTask{
		TaskColdStart:   {DecisionTimeout: DecisionCallTimeout, AttemptTimeout: 20 * time.Second, WalkTimeout: 40 * time.Second},
		TaskSummarize:   {DecisionTimeout: DecisionCallTimeout, AttemptTimeout: 200 * time.Second, WalkTimeout: CallCeiling},
		TaskAccountScan: {DecisionTimeout: DecisionCallTimeout, AttemptTimeout: 20 * time.Second},
	}
	for task, want := range cases {
		if got := v.Effective(task); got != want {
			t.Errorf("%s is sent %+v, want %+v", task, got, want)
		}
	}
}

// fakeClock is the router's clock in these tests; a slowClient moves it by the
// time each call takes.
type fakeClock struct{ at time.Time }

// slowClient takes `takes` on the fake clock, or its whole deadline when that
// is shorter, which it then reports as a deadline.
type slowClient struct {
	stubClient
	clock *fakeClock
	takes time.Duration
	text  string
	calls *int
}

func (s slowClient) Complete(ctx context.Context, _ model.Request) (model.Response, error) {
	*s.calls++
	limit := CallCeiling
	if deadline, ok := ctx.Deadline(); ok {
		limit = time.Until(deadline).Round(time.Second)
	}
	if s.takes > limit {
		s.clock.at = s.clock.at.Add(limit)
		return model.Response{}, fmt.Errorf("provider: %w", context.DeadlineExceeded)
	}
	s.clock.at = s.clock.at.Add(s.takes)
	return model.Response{Text: s.text, InputTokens: 1, OutputTokens: 1}, nil
}

// slowWalk drives one structured call of task, every answer of which the
// validator refuses, and returns the tries made and the time the user waited.
func slowWalk(t *testing.T, task Task, overrides TaskOverrides, clients map[Tier]slowClient) (int, time.Duration) {
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
	r.cacheOff = true
	r.SetTaskOverrides(overrides)
	_, _, err := r.CompleteStructured(wsCtx(), task, model.Request{}, func(string) error { return errors.New("refused") })
	if err == nil {
		t.Fatalf("%s: a walk the validator never accepts returned no error", task)
	}
	if len(fcs.recorded) != calls {
		t.Fatalf("%s: %d tries left %d rows, want one row for each try", task, calls, len(fcs.recorded))
	}
	if !isDeadline(err) {
		return calls, clock.at.Sub(time.Unix(0, 0))
	}
	if got := fcs.recorded[len(fcs.recorded)-1].ErrorSentinel; got != sentinelTimeout {
		t.Fatalf("%s: the call that spent its walk bound ends on %q, want %q", task, got, sentinelTimeout)
	}
	return calls, clock.at.Sub(time.Unix(0, 0))
}

// The walk that makes the most tries: cheap_cloud never answers, and premium
// answers in 30 s with text the validator refuses.
var hungThenRefused = map[Tier]slowClient{
	TierCheapCloud: {takes: time.Hour},
	TierPremium:    {takes: 30 * time.Second, text: "not the schema"},
}

func TestTheWalkBoundStopsASlowInteractiveCall(t *testing.T) {
	bound := TaskDefaults(TaskColdStart).WalkTimeout
	tries, waited := slowWalk(t, TaskColdStart, nil, hungThenRefused)
	if waited != bound || tries != 3 {
		t.Fatalf("cold_start made %d tries over %s, want 3 tries over its %s walk bound", tries, waited, bound)
	}
}

// A refused answer that lands as the bound runs out leaves no further walk,
// whether it ends the first walk or the schema retry.
func TestNoRetryStartsOnceTheWalkBoundIsSpent(t *testing.T) {
	refused := slowClient{takes: time.Minute, text: "not the schema"}
	for name, clients := range map[string]map[Tier]slowClient{
		"after the first walk": {TierCheapCloud: {takes: time.Hour}, TierPremium: refused},
		"after the retry":      {TierPremium: refused},
	} {
		tries, waited := slowWalk(t, TaskColdStart, TaskOverrides{TaskColdStart: {AttemptTimeoutMs: 60000}}, clients)
		if tries != 2 || waited != 2*time.Minute {
			t.Errorf("%s: cold_start made %d tries over %s, want 2 tries over its 2m0s walk bound", name, tries, waited)
		}
	}
}

// Five tries at today's limits: the walk a background task, and a waited task
// no record measures, still make.
func TestAnUnboundedWalkMakesEveryTry(t *testing.T) {
	for _, task := range []Task{TaskAccountScan, TaskTranscript} {
		tries, waited := slowWalk(t, task, nil, hungThenRefused)
		if want := 2*CallCeiling + 3*30*time.Second; tries != 5 || waited != want {
			t.Errorf("%s made %d tries over %s, want 5 over %s", task, tries, waited, want)
		}
	}
}
