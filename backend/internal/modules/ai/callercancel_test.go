// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A caller that stopped waiting is filed as canceled, whatever its provider's
// client made of the cancellation. A deadline stays a timeout, and an outcome
// the attempt reached keeps its own name.
func TestTheCallersOwnCancellationIsFiledAsCanceled(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer stop()
	live := context.Background()
	cases := map[string]struct {
		ctx  context.Context
		err  error
		want string
	}{
		"a cancellation the client wrapped":       {canceled, fmt.Errorf("post: %w", context.Canceled), sentinelCanceled},
		"a cancellation the client reported raw":  {canceled, errors.New("read tcp: connection reset by peer"), sentinelCanceled},
		"an attempt deadline after the cancel":    {canceled, fmt.Errorf("post: %w", context.DeadlineExceeded), sentinelCanceled},
		"a quota the cancelled attempt reached":   {canceled, fmt.Errorf("wrap: %w", ErrProviderQuota), "provider_quota"},
		"an answer served before the cancel":      {canceled, errMeteringFailed, sentinelMeteringFailed},
		"the caller's own deadline":               {expired, fmt.Errorf("post: %w", context.DeadlineExceeded), sentinelTimeout},
		"a broken provider with the caller alive": {live, errors.New("read tcp: connection reset by peer"), sentinelProviderError},
		"success after the cancel":                {canceled, nil, ""},
	}
	for name, c := range cases {
		if got := sentinelFor(c.ctx, c.err); got != c.want {
			t.Errorf("%s: filed as %q, want %q", name, got, c.want)
		}
	}
}

// cancelThenFail cancels its caller and fails the way an HTTP client does when
// the connection under it goes: with an error that names no context.
type cancelThenFail struct {
	stubClient
	cancel context.CancelFunc
}

func (c cancelThenFail) Complete(context.Context, model.Request) (model.Response, error) {
	c.cancel()
	return model.Response{}, errors.New("read tcp: connection reset by peer")
}

func TestACallerWhoCanceledGetsNoFurtherRung(t *testing.T) {
	ctx, cancel := context.WithCancel(wsContext(t))
	defer cancel()
	premium := 0
	store := &fakeCallStore{}
	r := assembleRouter(
		map[Tier]model.Client{
			TierCheapCloud: cancelThenFail{cancel: cancel},
			TierPremium:    countingClient{resp: model.Response{Text: "nobody reads this"}, calls: &premium},
		},
		nil, ProfileCloudFrontier, stubMeter{}, unlimitedBudget{}, store,
		map[Tier]routeMeta{TierCheapCloud: {provider: "openai", model: "gpt-x"}}, false, nil)
	if _, _, err := r.serveCompletion(ctx, TaskColdStart, []Tier{TierCheapCloud, TierPremium}, model.Request{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled call answered %v, want the cancellation", err)
	}
	if premium != 0 {
		t.Errorf("the rung above was asked %d time(s) after the caller left", premium)
	}
	if len(store.recorded) != 1 || store.recorded[0].ErrorSentinel != sentinelCanceled {
		t.Fatalf("want one attempt filed as canceled, got %+v", store.recorded)
	}
}

// A caller gone before the call starts costs no model call at all, and still
// leaves its one canceled row.
func TestACallWhoseCallerAlreadyLeftAsksNoModel(t *testing.T) {
	ctx, cancel := context.WithCancel(wsContext(t))
	cancel()
	cheap := 0
	store := &fakeCallStore{}
	r := assembleRouter(
		map[Tier]model.Client{TierCheapCloud: countingClient{resp: model.Response{Text: "nobody reads this"}, calls: &cheap}},
		nil, ProfileCloudFrontier, stubMeter{}, unlimitedBudget{}, store,
		map[Tier]routeMeta{TierCheapCloud: {provider: "openai", model: "gpt-x"}}, false, nil)
	if _, _, err := r.serveCompletion(ctx, TaskColdStart, []Tier{TierCheapCloud}, model.Request{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("a call whose caller had left answered %v, want the cancellation", err)
	}
	if cheap != 0 {
		t.Errorf("the model was asked %d time(s) for a caller already gone", cheap)
	}
	if len(store.recorded) != 1 || store.recorded[0].ErrorSentinel != sentinelCanceled {
		t.Fatalf("want one row filed as canceled, got %+v", store.recorded)
	}
}

// cancelOnInvalid answers text no validator takes, and cancels its caller on
// call cancelOn: a page closed while a malformed answer came back.
type cancelOnInvalid struct {
	stubClient
	cancel   context.CancelFunc
	cancelOn int
	calls    *int
}

func (c cancelOnInvalid) Complete(context.Context, model.Request) (model.Response, error) {
	*c.calls++
	if *c.calls == c.cancelOn {
		c.cancel()
	}
	return model.Response{Text: "not json", InputTokens: 10, OutputTokens: 2}, nil
}

// Canceled during the first answer, the call makes no schema retry; during the
// retry, no escalation. Either would record an attempt that asked no model.
func TestAStructuredCallStopsRetryingWhenItsCallerCanceled(t *testing.T) {
	for _, cancelOn := range []int{1, 2} {
		ctx, cancel := context.WithCancel(wsContext(t))
		calls := 0
		client := cancelOnInvalid{cancel: cancel, cancelOn: cancelOn, calls: &calls}
		store := &fakeCallStore{}
		r := assembleRouter(map[Tier]model.Client{TierCheapCloud: client, TierPremium: client},
			nil, ProfileEUHosted, &memMeter{}, DefaultMonthlyTokens, store,
			map[Tier]routeMeta{TierCheapCloud: {provider: "openai", model: "gpt-x"}}, false, nil)
		_, _, err := r.CompleteStructured(ctx, TaskColdStart, structuredReq(), jsonObjectValidator)
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled on call %d: answered %v, want the cancellation", cancelOn, err)
		}
		if calls != cancelOn || len(store.recorded) != cancelOn {
			t.Fatalf("canceled on call %d: %d model calls and %d recorded attempts, want %d of each",
				cancelOn, calls, len(store.recorded), cancelOn)
		}
		// The refused answer is the terminal; read as answered it would put a
		// call nobody waited for in the answered bucket.
		if terminal := store.recorded[cancelOn-1]; !terminal.IsTerminal || terminal.ErrorSentinel != sentinelCanceled {
			t.Errorf("canceled on call %d: the terminal is %+v, want it filed as canceled", cancelOn, terminal)
		}
	}
}

// cancelingDecider cancels its caller mid-decision and reports it as the
// decision wire does.
type cancelingDecider struct{ cancel context.CancelFunc }

func (d cancelingDecider) Decide(ctx context.Context, _ decision.Request) (decision.Response, error) {
	d.cancel()
	<-ctx.Done()
	return decision.Response{}, ctx.Err()
}

func TestADecisionWhoseCallerCanceledIsNotHandedToTheLadder(t *testing.T) {
	ctx, cancel := context.WithCancel(wsContext(t))
	defer cancel()
	ladderCalls := 0
	f := newDecideFixtureOver(t, cancelingDecider{cancel: cancel}, 0,
		countingClient{resp: model.Response{Text: "ladder answer"}, calls: &ladderCalls})
	_, _, err := f.router.Decide(ctx, TaskSiteTriage, "triage", decision.Request{}, triageLLMRequest, acceptAnything, floorGate)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a decision whose caller left answered %v, want the cancellation", err)
	}
	if ladderCalls != 0 {
		t.Errorf("the ladder was asked %d time(s) after the caller left", ladderCalls)
	}
	if len(f.store.recorded) != 1 || f.store.recorded[0].ErrorSentinel != sentinelCanceled {
		t.Fatalf("want the decision attempt alone, filed as canceled, got %+v", f.store.recorded)
	}
}

// unsureThenCancel answers below the floor and then loses its caller.
type unsureThenCancel struct{ cancel context.CancelFunc }

func (d unsureThenCancel) Decide(context.Context, decision.Request) (decision.Response, error) {
	d.cancel()
	return answered("question", 0.3).resp, nil
}

// A decision too unsure to stand would hand on to the ladder. With the caller
// gone it ends there, and its row says why rather than reading as decided.
func TestAnUnsureDecisionWhoseCallerLeftIsFiledAsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(wsContext(t))
	defer cancel()
	ladderCalls := 0
	f := newDecideFixtureOver(t, unsureThenCancel{cancel: cancel}, 0,
		countingClient{resp: model.Response{Text: "ladder answer"}, calls: &ladderCalls})
	_, _, err := f.router.Decide(ctx, TaskSiteTriage, "triage", decision.Request{}, triageLLMRequest, acceptAnything, floorGate)
	if !errors.Is(err, context.Canceled) || ladderCalls != 0 {
		t.Fatalf("answered %v after %d ladder call(s), want the cancellation and none", err, ladderCalls)
	}
	if len(f.store.recorded) != 1 || f.store.recorded[0].ErrorSentinel != sentinelCanceled {
		t.Fatalf("want the decision row alone, filed as canceled, got %+v", f.store.recorded)
	}
}
