// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// untracked returns the adapter beneath a health wrapper, for a test that
// needs the concrete client rather than the Client interface.
func untracked(c model.Client) model.Client {
	if t, ok := c.(*trackedClient); ok {
		return t.Client
	}
	return c
}

type healthClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *healthClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *healthClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newClock() *healthClock { return &healthClock{t: time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)} }

func TestAProviderFaultIsClassifiedFromItsSentinelOrItsTransport(t *testing.T) {
	t.Parallel()
	dial := &url.Error{Op: "Post", URL: "https://x", Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}
	for name, tc := range map[string]struct {
		err  error
		want failureKind
	}{
		"nil is no fault":                   {nil, failNone},
		"the caller cancelling is no fault": {context.Canceled, failNone},
		"a refused message is no fault":     {errors.New("ai: model returned unreadable text"), failNone},
		"an empty account":                  {fmt.Errorf("wrap: %w", ErrProviderQuota), failCredit},
		"a rejected key":                    {fmt.Errorf("wrap: %w", ErrProviderUnauthorized), failAuth},
		"a server fault":                    {fmt.Errorf("wrap: %w", ErrProviderUnavailable), failServer},
		"a refused connection":              {dial, failUnreachable},
		"a name that does not resolve":      {&net.DNSError{Name: "x", Err: "no such host"}, failUnreachable},
		"a deadline":                        {fmt.Errorf("wrap: %w", context.DeadlineExceeded), failTimeout},
	} {
		if got := classifyFailure(tc.err); got != tc.want {
			t.Errorf("%s: classified %v, want %v", name, got, tc.want)
		}
	}
}

func TestAnHTTPStatusIsTaggedOnlyWhenTheProviderItselfIsFailing(t *testing.T) {
	t.Parallel()
	base := errors.New("http")
	for status, want := range map[int]error{
		http.StatusPaymentRequired:       ErrProviderQuota,
		http.StatusUnauthorized:          ErrProviderUnauthorized,
		http.StatusForbidden:             nil,
		http.StatusBadGateway:            ErrProviderUnavailable,
		http.StatusInternalServerError:   ErrProviderUnavailable,
		http.StatusBadRequest:            nil,
		http.StatusNotFound:              nil,
		http.StatusUnprocessableEntity:   nil,
		http.StatusRequestEntityTooLarge: nil,
	} {
		got := providerRefusal(&http.Response{StatusCode: status}, "", base)
		if want == nil {
			if got != base {
				t.Errorf("status %d: %v, want the error as it came", status, got)
			}
			continue
		}
		if !errors.Is(got, want) || !errors.Is(got, base) {
			t.Errorf("status %d: %v, want %v wrapping the cause", status, got, want)
		}
	}
}

func TestAnEmptyBalanceOrRejectedKeyIsRecognisedByItsTextUnderStatusesThatAlsoMeanOtherThings(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		status int
		text   string
		want   error
	}{
		"anthropic empty balance":             {http.StatusBadRequest, "anthropic: invalid_request_error: Your credit balance is too low to access the API (http 400)", ErrProviderQuota},
		"gemini key not valid":                {http.StatusBadRequest, "gemini: INVALID_ARGUMENT: API key not valid. Please pass a valid API key.", ErrProviderUnauthorized},
		"a revoked key under 403":             {http.StatusForbidden, "vendor: PERMISSION_DENIED: your API key was reported as leaked", ErrProviderUnauthorized},
		"a moderation flag under 403":         {http.StatusForbidden, "openrouter: this input requires moderation and was flagged", nil},
		"a model not enabled under 403":       {http.StatusForbidden, "vertex: PERMISSION_DENIED: the model is not enabled for this project", nil},
		"a permission refusal naming the key": {http.StatusForbidden, "anthropic: permission_error: Your API key does not have permission to use the specified resource.", nil},
		"an ordinary bad request":             {http.StatusBadRequest, "openai: invalid_request_error: max_tokens is too large", nil},
	} {
		got := providerFaultOf(tc.status, errors.New(tc.text))
		if tc.want == nil && errors.Unwrap(got) != nil {
			t.Errorf("%s: %v was tagged, want it returned as it came", name, got)
		}
		if tc.want != nil && !errors.Is(got, tc.want) {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

func TestOneEmptyAccountOrRejectedKeyBlocksTheProviderAndAReprobeIsFifteenMinutesOut(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]model.ProviderHealth{
		ErrProviderQuota:        model.HealthOutOfCredit,
		ErrProviderUnauthorized: model.HealthUnauthorized,
	} {
		clock := newClock()
		tr := newProviderTracker(clock.now)
		tr.observe(admission{}, err)
		st := tr.current()
		if st.Health != want || !st.Since.Equal(clock.now()) || !st.RetryAfter.Equal(clock.now().Add(accountReprobe)) {
			t.Errorf("%v: status %+v, want %s with a 15 minute reprobe", err, st, want)
		}
		if _, _, ok := tr.admit(); ok {
			t.Errorf("%v: a call was admitted while the provider is blocked", err)
		}
	}
}

func TestServerFaultsTripOnlyAfterARunAndASuccessEndsTheRun(t *testing.T) {
	t.Parallel()
	tr := newProviderTracker(newClock().now)
	fault := fmt.Errorf("%w", ErrProviderUnavailable)
	tr.observe(admission{}, fault)
	tr.observe(admission{}, fault)
	tr.observe(admission{}, nil)
	tr.observe(admission{}, fault)
	tr.observe(admission{}, fault)
	if h := tr.current().Health; h != model.HealthOK {
		t.Fatalf("health %s after two faults, a success and two more, want ok", h)
	}
	tr.observe(admission{}, fault)
	if h := tr.current().Health; h != model.HealthDown {
		t.Errorf("health %s after three faults in a row, want down", h)
	}
}

func TestTimeoutsDegradeTheProviderWithoutBlockingIt(t *testing.T) {
	t.Parallel()
	tr := newProviderTracker(newClock().now)
	for range consecutiveToTrip {
		tr.observe(admission{}, context.DeadlineExceeded)
	}
	st := tr.current()
	if st.Health != model.HealthDegraded || st.Health.Blocking() {
		t.Errorf("status %+v, want degraded and not blocking", st)
	}
	if _, _, ok := tr.admit(); !ok {
		t.Error("a degraded provider refused a call")
	}
	tr.observe(admission{}, nil)
	if h := tr.current().Health; h != model.HealthOK {
		t.Errorf("health %s after a success, want ok", h)
	}
}

func TestAnUnreachableProviderIsDownAtOnce(t *testing.T) {
	t.Parallel()
	tr := newProviderTracker(newClock().now)
	tr.observe(admission{}, &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED})
	if h := tr.current().Health; h != model.HealthDown {
		t.Errorf("health %s after one refused connection, want down", h)
	}
}

func TestAMessageTheProviderRefusedProvesItIsAnswering(t *testing.T) {
	t.Parallel()
	clock := newClock()
	tr := newProviderTracker(clock.now)
	tr.observe(admission{}, ErrProviderQuota)
	clock.advance(accountReprobe)
	_, probe, ok := tr.admit()
	if !ok {
		t.Fatal("the probe was refused")
	}
	tr.observe(probe, errors.New("ai: provider refused this request"))
	if h := tr.current().Health; h != model.HealthOK {
		t.Errorf("health %s after the provider answered a probe, want ok", h)
	}
}

func TestAProbeInFlightRefusesOthersWithAMomentStillAheadOfThem(t *testing.T) {
	t.Parallel()
	clock := newClock()
	tr := newProviderTracker(clock.now)
	tr.observe(admission{}, ErrProviderUnauthorized)
	clock.advance(accountReprobe)
	if _, _, ok := tr.admit(); !ok {
		t.Fatal("the probe was refused")
	}
	st, _, ok := tr.admit()
	if ok || !st.RetryAfter.After(clock.now()) {
		t.Errorf("a second caller got admitted=%v with retry %v, want a refusal with a moment after now", ok, st.RetryAfter)
	}
	clock.advance(CallCeiling - time.Second)
	if _, _, ok := tr.admit(); ok {
		t.Error("a second probe started while the first could still be running")
	}
	clock.advance(time.Second)
	if _, _, ok := tr.admit(); !ok {
		t.Error("a probe that never reported held the slot past the longest call")
	}
}

func TestAProbeTheCallerCancelsFreesItsSlotWithoutCounting(t *testing.T) {
	t.Parallel()
	clock := newClock()
	tr := newProviderTracker(clock.now)
	tr.observe(admission{}, ErrProviderQuota)
	clock.advance(accountReprobe)
	_, probe, ok := tr.admit()
	if !ok {
		t.Fatal("the probe was refused")
	}
	tr.observe(probe, context.Canceled)
	if h := tr.current().Health; h != model.HealthOutOfCredit {
		t.Errorf("health %s after a cancelled probe, want it unchanged", h)
	}
	if _, _, ok := tr.admit(); !ok {
		t.Error("a cancelled probe kept the slot")
	}
}

func TestAnAccountFaultKeepsItsLabelWhenAProbeFailsAnotherWay(t *testing.T) {
	t.Parallel()
	clock := newClock()
	tr := newProviderTracker(clock.now)
	tr.observe(admission{}, ErrProviderQuota)
	clock.advance(accountReprobe)
	_, probe, _ := tr.admit()
	tr.observe(probe, ErrProviderUnavailable)
	if st := tr.current(); st.Health != model.HealthOutOfCredit || !st.RetryAfter.Equal(clock.now().Add(accountReprobe)) {
		t.Errorf("status %+v, want out_of_credit re-armed for the account reprobe", st)
	}
}

func TestExactlyOneProbeIsAdmittedOnceTheRetryMomentPasses(t *testing.T) {
	t.Parallel()
	clock := newClock()
	tr := newProviderTracker(clock.now)
	tr.observe(admission{}, ErrProviderUnauthorized)
	clock.advance(accountReprobe - time.Second)
	if _, _, ok := tr.admit(); ok {
		t.Fatal("a call was admitted before the retry moment")
	}
	clock.advance(time.Second)
	admitted := 0
	for range 5 {
		if _, _, ok := tr.admit(); ok {
			admitted++
		}
	}
	if admitted != 1 {
		t.Errorf("%d probes admitted, want exactly 1", admitted)
	}
}

func TestAFailedProbeKeepsSinceAndBacksOffADownProvider(t *testing.T) {
	t.Parallel()
	clock := newClock()
	tr := newProviderTracker(clock.now)
	start := clock.now()
	tr.observe(admission{}, &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED})
	var waits []time.Duration
	for range 5 {
		st := tr.current()
		waits = append(waits, st.RetryAfter.Sub(clock.now()))
		clock.advance(st.RetryAfter.Sub(clock.now()))
		_, probe, ok := tr.admit()
		if !ok {
			t.Fatal("the probe was refused")
		}
		tr.observe(probe, &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED})
	}
	if got := tr.current().Since; !got.Equal(start) {
		t.Errorf("since moved to %v, want it kept at %v", got, start)
	}
	want := []time.Duration{downProbeFloor, 2 * downProbeFloor, 4 * downProbeFloor, 8 * downProbeFloor, downProbeCap}
	for i := range want {
		if waits[i] != want[i] {
			t.Errorf("wait %d = %v, want %v", i, waits[i], want[i])
		}
	}
}

func TestForgettingAProviderClearsItsFailuresSoTheNextCallProbes(t *testing.T) {
	t.Parallel()
	book := newProviderBook(newClock().now)
	book.tracker("openai").observe(admission{}, ErrProviderQuota)
	if len(book.snapshot()) != 1 {
		t.Fatal("the blocked provider is missing from the snapshot")
	}
	book.forget("openai")
	if got := book.snapshot(); len(got) != 0 {
		t.Errorf("snapshot %v after forgetting, want none", got)
	}
	if _, _, ok := book.tracker("openai").admit(); !ok {
		t.Error("a forgotten provider refused its next call")
	}
}

func TestTheSnapshotListsOnlyProvidersThatAreNotOKInNameOrder(t *testing.T) {
	t.Parallel()
	book := newProviderBook(newClock().now)
	book.tracker("openai").observe(admission{}, ErrProviderQuota)
	book.tracker("anthropic").observe(admission{}, ErrProviderUnauthorized)
	book.tracker("gemini").observe(admission{}, nil)
	got := book.snapshot()
	if len(got) != 2 || got[0].Provider != "anthropic" || got[1].Provider != "openai" {
		t.Errorf("snapshot %+v, want anthropic then openai", got)
	}
}

type faultClient struct {
	model.NoHealth
	err   error
	calls int
}

func (c *faultClient) Complete(context.Context, model.Request) (model.Response, error) {
	c.calls++
	return model.Response{Text: "ok"}, c.err
}

func (c *faultClient) Stream(context.Context, model.Request) (model.TokenStream, error) {
	c.calls++
	return nil, c.err
}

func (c *faultClient) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	c.calls++
	return model.Embeddings{}, c.err
}

func (c *faultClient) Caps() model.Capabilities { return model.Capabilities{} }

func TestABlockedProviderIsNotCalledAndSaysWhenItMayBeTriedAgain(t *testing.T) {
	t.Parallel()
	clock := newClock()
	inner := &faultClient{err: ErrProviderQuota}
	client := trackClient(inner, "openai", newProviderBook(clock.now))
	ctx := context.Background()
	if _, err := client.Complete(ctx, model.Request{}); !errors.Is(err, ErrProviderQuota) {
		t.Fatalf("first call: %v, want the provider's refusal", err)
	}
	for name, call := range map[string]func() error{
		"complete": func() error { _, err := client.Complete(ctx, model.Request{}); return err },
		"stream":   func() error { _, err := client.Stream(ctx, model.Request{}); return err },
		"embed":    func() error { _, err := client.Embed(ctx, model.EmbedRequest{}); return err },
	} {
		err := call()
		var down *ProviderDownError
		if !errors.Is(err, ErrProviderDown) || !errors.As(err, &down) {
			t.Fatalf("%s: %v, want ErrProviderDown", name, err)
		}
		if down.Provider != "openai" || down.Health != model.HealthOutOfCredit || !down.RetryAfter.Equal(clock.now().Add(accountReprobe)) {
			t.Errorf("%s: %+v names the wrong provider, cause or moment", name, down)
		}
	}
	if inner.calls != 1 {
		t.Errorf("provider called %d times, want 1: a blocked provider must not be called", inner.calls)
	}
	if client.Health().Health != model.HealthOutOfCredit {
		t.Errorf("health %s, want out_of_credit", client.Health().Health)
	}
}

func TestEveryDeferralSaysWhenTheItemMayBeTriedAgain(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for name, err := range map[string]error{
		"budget":   &BudgetDeferralError{Task: TaskEnrich, NextAttemptAt: at},
		"provider": fmt.Errorf("wrap: %w", &ProviderDownError{RetryAfter: at}),
	} {
		if !IsDeferral(err) {
			t.Errorf("%s: not a deferral", name)
		}
		if got, ok := DeferredUntil(err); !ok || !got.Equal(at) {
			t.Errorf("%s: deferred until %v (%v), want %v", name, got, ok, at)
		}
	}
	if IsDeferral(ErrProviderQuota) || IsDeferral(errors.New("x")) {
		t.Error("a provider refusal or a plain error was read as a deferral")
	}
	if _, ok := DeferredUntil(errors.New("x")); ok {
		t.Error("a plain error was given a retry moment")
	}
}

func TestTheRouterRefusesACallWhoseEveryRungIsBlockedWithoutCallingOrTracing(t *testing.T) {
	t.Parallel()
	clock := newClock()
	book := newProviderBook(clock.now)
	inner := &faultClient{err: ErrProviderUnauthorized}
	blocked := trackClient(inner, "openai", book)
	blocked.(*trackedClient).tracker.observe(admission{}, ErrProviderUnauthorized)
	router := assembleRouter(map[Tier]model.Client{TierCheapCloud: blocked}, blocked, ProfileCloudFrontier,
		&memoryMeter{}, StaticBudget(1<<40), nil, nil, false, nil)
	router.now = clock.now
	_, _, err := router.Complete(workspaceCtx(), TaskSummarize, model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if !errors.Is(err, ErrProviderDown) {
		t.Fatalf("error %v, want ErrProviderDown", err)
	}
	if inner.calls != 0 {
		t.Errorf("provider called %d times, want none", inner.calls)
	}
}

// Not parallel: it writes the process-wide book, which parallel tests share.
func TestARebindClearsWhatTheOperatorJustFixed(t *testing.T) {
	cfg := RoutingConfig{
		Profile:    ProfileCloudFrontier,
		Tiers:      map[Tier]ProviderConfig{TierCheapCloud: {Provider: ProviderFake}},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: ProviderFake}},
	}
	router, err := NewLocalRouter(cfg)
	if err != nil {
		t.Fatalf("building the router: %v", err)
	}
	sharedProviderHealth.tracker(ProviderFake).observe(admission{}, ErrProviderUnauthorized)
	if err := router.Rebind(cfg); err != nil {
		t.Fatalf("rebinding: %v", err)
	}
	if got := sharedProviderHealth.tracker(ProviderFake).current().Health; got != model.HealthOK {
		t.Errorf("health %s after a rebind, want ok", got)
	}
}

// ladderOf binds two tiers to two providers, each reporting into its own book,
// so a test can stand one provider's outage beside the other's answer.
func ladderOf(cheap, premium model.Client) *Router {
	return testRouter(map[Tier]model.Client{TierCheapCloud: cheap, TierPremium: premium},
		&memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)
}

// ladderOn is ladderOf with the router reading the same hand-driven clock as
// the trackers beneath it, so a retry moment means the same thing to both.
func ladderOn(clock *healthClock, cheap, premium model.Client) *Router {
	r := ladderOf(cheap, premium)
	r.now = clock.now
	return r
}

func TestTheCallThatTripsAnEmptyAccountIsRefundedLikeTheNext(t *testing.T) {
	t.Parallel()
	book := newProviderBook(newClock().now)
	empty := trackClient(&faultClient{err: ErrProviderQuota}, "openai", book)
	r := ladderOf(empty, empty)
	_, _, err := r.Complete(wsContext(t), TaskSummarize, model.Request{Messages: []model.Message{{Role: "user", Content: "x"}}})
	var down *ProviderDownError
	if !errors.As(err, &down) || down.Health != model.HealthOutOfCredit {
		t.Fatalf("error %v, want a deferral for an out-of-credit provider", err)
	}
	if !errors.Is(err, ErrProviderQuota) || !IsDeferral(err) {
		t.Errorf("error %v lost the refusal that tripped the provider, or is not a deferral", err)
	}
}

func TestTheCallThatTripsARejectedKeyIsRefundedWhenTheWalkEndsOnIt(t *testing.T) {
	t.Parallel()
	book := newProviderBook(newClock().now)
	rejected := trackClient(&faultClient{err: ErrProviderUnauthorized}, "openai", book)
	r := ladderOf(rejected, rejected)
	_, _, err := r.Complete(wsContext(t), TaskSummarize, model.Request{Messages: []model.Message{{Role: "user", Content: "x"}}})
	if !IsDeferral(err) || !errors.Is(err, ErrProviderUnauthorized) {
		t.Errorf("error %v, want a deferral that keeps the rejected-key cause", err)
	}
}

func TestABlockedRungNeverReplacesWhatACalledRungAnswered(t *testing.T) {
	t.Parallel()
	book := newProviderBook(newClock().now)
	blocked := trackClient(&faultClient{}, "openai", book)
	blocked.(*trackedClient).tracker.observe(admission{}, ErrProviderQuota)
	poison := errors.New("provider refused this message")
	r := ladderOf(&faultClient{err: poison}, blocked)
	_, _, err := r.Complete(wsContext(t), TaskSummarize, model.Request{Messages: []model.Message{{Role: "user", Content: "x"}}})
	if IsDeferral(err) || !errors.Is(err, poison) {
		t.Errorf("error %v, want the called rung's own failure, charged to the message", err)
	}
}

func TestAnEmbeddingForABlockedProviderIsRefusedUntracedAndNotWrappedAsAFailedLane(t *testing.T) {
	t.Parallel()
	book := newProviderBook(newClock().now)
	blocked := trackClient(&faultClient{}, "openai", book)
	blocked.(*trackedClient).tracker.observe(admission{}, ErrProviderUnauthorized)
	r := assembleRouter(map[Tier]model.Client{TierCheapCloud: NewFakeClient()}, blocked, ProfileEUHosted,
		&memoryMeter{}, StaticBudget(1<<40), nil, nil, false, nil)
	_, err := r.Embed(wsContext(t), model.EmbedRequest{Inputs: []string{"x"}})
	if !errors.Is(err, ErrProviderDown) || errors.Is(err, ErrEmbedLaneFailed) {
		t.Errorf("error %v, want ErrProviderDown and not an embed-lane failure", err)
	}
}

func TestABindingNamesTheProvidersItsTiersUse(t *testing.T) {
	t.Parallel()
	b := binding{routeMeta: map[Tier]routeMeta{TierCheapCloud: {provider: "openai"}, TierPremium: {provider: "openai"}, TierEmbedLane: {provider: "gemini"}}}
	got := b.providers()
	if len(got) != 2 || got[0] != "gemini" || got[1] != "openai" {
		t.Errorf("providers %v, want gemini and openai once each", got)
	}
}

func TestAFailedProbeOrAnUnreachableHostRefundsTheItemTheWalkEndedOn(t *testing.T) {
	t.Parallel()
	clock := newClock()
	dial := &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}
	client := trackClient(&faultClient{err: dial}, "openai", newProviderBook(clock.now))
	r := ladderOn(clock, client, client)
	ask := func() error {
		_, _, err := r.Complete(wsContext(t), TaskSummarize, model.Request{Messages: []model.Message{{Role: "user", Content: "x"}}})
		return err
	}
	if err := ask(); !IsDeferral(err) || !errors.Is(err, dial) {
		t.Errorf("the call that found the host unreachable: %v, want a deferral keeping the cause", err)
	}
	clock.advance(downProbeCap)
	if err := ask(); !IsDeferral(err) {
		t.Errorf("a failed probe: %v, want a deferral, not a charge to the item that probed", err)
	}
}

func TestACallRefusedBecauseTheProviderIsBlockedHasNothingToTrace(t *testing.T) {
	t.Parallel()
	if !refusedUncalled(&ProviderDownError{}) {
		t.Error("a refusal with no failure behind it would be traced as a provider error")
	}
	if refusedUncalled(&ProviderDownError{Cause: ErrProviderQuota}) {
		t.Error("the call that tripped the provider made a real call and must be traced")
	}
	if refusedUncalled(errors.New("x")) {
		t.Error("an ordinary error was read as an untraced refusal")
	}
}

func TestACallAdmittedBeforeTheBlockNeitherReleasesTheProbeNorRestoresTheProvider(t *testing.T) {
	t.Parallel()
	clock := newClock()
	tr := newProviderTracker(clock.now)
	_, early, _ := tr.admit()
	tr.observe(admission{}, ErrProviderQuota)
	clock.advance(accountReprobe)
	_, probe, ok := tr.admit()
	if !ok || !probe.probe {
		t.Fatal("the probe was not admitted")
	}
	if _, blocked := tr.observe(early, nil); blocked || tr.current().Health != model.HealthOutOfCredit {
		t.Fatalf("a call from before the block restored the provider: %+v", tr.current())
	}
	if _, _, again := tr.admit(); again {
		t.Error("a call from before the block released the probe's slot")
	}
	tr.observe(probe, nil)
	if tr.current().Health != model.HealthOK {
		t.Errorf("health %s after the probe answered, want ok", tr.current().Health)
	}
}

func TestOnlyTheCallThatBlockedTheProviderIsRefundedForIt(t *testing.T) {
	t.Parallel()
	clock := newClock()
	tr := newProviderTracker(clock.now)
	_, a, _ := tr.admit()
	if _, blocked := tr.observe(a, errors.New("provider refused this message")); blocked {
		t.Error("a message the provider refused was reported as blocking it")
	}
	tr.observe(admission{}, ErrProviderQuota)
	if _, blocked := tr.observe(a, errors.New("provider refused this message")); blocked {
		t.Error("a message refused earlier was dressed as the outage a concurrent call caused")
	}
	reader := newProviderTracker(clock.now)
	if st, blocked := reader.observe(admission{}, ErrProviderQuota); !blocked || st.Health != model.HealthOutOfCredit {
		t.Errorf("the call that blocked the provider got %+v blocked=%v, want to be told so", st, blocked)
	}
}

func TestAnAuthenticationPolicyRefusalIsTheMessagesOwn(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden} {
		got := providerFaultOf(status, errors.New("vendor: this request's authentication method is not allowed by policy"))
		if errors.Is(got, ErrProviderUnauthorized) {
			t.Errorf("status %d: an authentication-policy refusal blocked the provider", status)
		}
	}
}

type streamClient struct {
	model.NoHealth
	stream model.TokenStream
}

func (c *streamClient) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, nil
}

func (c *streamClient) Stream(context.Context, model.Request) (model.TokenStream, error) {
	return c.stream, nil
}

func (c *streamClient) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	return model.Embeddings{}, nil
}
func (c *streamClient) Caps() model.Capabilities { return model.Capabilities{} }

type scriptedStream struct {
	err    error
	closed bool
}

func (s *scriptedStream) Next(context.Context) (string, bool, error) { return "", false, s.err }
func (s *scriptedStream) Close() error                               { s.closed = true; return nil }

func TestAStreamThatFailsMidResponseBlocksItsProvider(t *testing.T) {
	t.Parallel()
	clock := newClock()
	book := newProviderBook(clock.now)
	stream := &scriptedStream{err: ErrProviderQuota}
	client := trackClient(&streamClient{stream: stream}, "openai", book)
	opened, err := client.Stream(context.Background(), model.Request{})
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}
	if h := client.Health().Health; h != model.HealthOK {
		t.Fatalf("health %s before the stream ended, want ok: an opened stream has not answered yet", h)
	}
	_, _, err = opened.Next(context.Background())
	if !IsDeferral(err) || !errors.Is(err, ErrProviderQuota) {
		t.Errorf("the terminal error %v, want a deferral that keeps the refusal", err)
	}
	if h := client.Health().Health; h != model.HealthOutOfCredit {
		t.Errorf("health %s after the stream failed, want out_of_credit", h)
	}
}

func TestAStreamClosedUnfinishedFreesTheProbeSlotWithoutCounting(t *testing.T) {
	t.Parallel()
	clock := newClock()
	book := newProviderBook(clock.now)
	tracker := book.tracker("openai")
	tracker.observe(admission{}, ErrProviderQuota)
	clock.advance(accountReprobe)
	client := trackClient(&streamClient{stream: &scriptedStream{}}, "openai", book)
	opened, err := client.Stream(context.Background(), model.Request{})
	if err != nil {
		t.Fatalf("the probe stream was refused: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}
	if h := client.Health().Health; h != model.HealthOutOfCredit {
		t.Errorf("health %s after an abandoned probe, want it unchanged", h)
	}
	if _, _, ok := tracker.admit(); !ok {
		t.Error("an abandoned probe stream kept the slot")
	}
}

func TestACachedAnswerIsServedWhileTheProviderIsBlocked(t *testing.T) {
	t.Parallel()
	clock := newClock()
	book := newProviderBook(clock.now)
	inner := &faultClient{}
	client := trackClient(inner, "openai", book)
	r := ladderOn(clock, client, client)
	ctx := wsContext(t)
	ask := func() error {
		_, _, err := r.Complete(ctx, TaskSummarize, model.Request{Messages: []model.Message{{Role: "user", Content: "same question"}}})
		return err
	}
	if err := ask(); err != nil {
		t.Fatalf("the first call: %v", err)
	}
	client.(*trackedClient).tracker.observe(admission{}, ErrProviderQuota)
	if err := ask(); err != nil {
		t.Errorf("a cached answer was withheld by an outage: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("provider called %d times, want the one that filled the cache", inner.calls)
	}
}

func TestAnEmptyAccountEndsTheWalkInsteadOfBillingARungAbove(t *testing.T) {
	t.Parallel()
	clock := newClock()
	book := newProviderBook(clock.now)
	empty := trackClient(&faultClient{}, "openai", book)
	empty.(*trackedClient).tracker.observe(admission{}, ErrProviderQuota)
	premium := &faultClient{}
	r := ladderOn(clock, empty, trackClient(premium, "gemini", newProviderBook(clock.now)))
	_, _, err := r.Complete(wsContext(t), TaskSummarize, model.Request{Messages: []model.Message{{Role: "user", Content: "x"}}})
	if !IsDeferral(err) || premium.calls != 0 {
		t.Errorf("error %v with the premium rung called %d times, want a deferral and no silent fallback", err, premium.calls)
	}
}

func TestASkippedWalkDefersOnlyUntilTheFirstProviderMayAnswer(t *testing.T) {
	t.Parallel()
	soon := &ProviderDownError{Health: model.HealthDown, RetryAfter: time.Date(2026, 10, 5, 9, 1, 0, 0, time.UTC)}
	later := &ProviderDownError{Health: model.HealthDown, RetryAfter: time.Date(2026, 10, 5, 9, 15, 0, 0, time.UTC)}
	for name, got := range map[string]error{"later first": earlier(later, soon), "sooner first": earlier(soon, later)} {
		if until, _ := DeferredUntil(got); !until.Equal(soon.RetryAfter) {
			t.Errorf("%s: deferred until %v, want the sooner %v", name, until, soon.RetryAfter)
		}
	}
}
