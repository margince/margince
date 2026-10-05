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
		http.StatusForbidden:             ErrProviderUnauthorized,
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

func TestOneEmptyAccountOrRejectedKeyBlocksTheProviderAndAReprobeIsFifteenMinutesOut(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]model.ProviderHealth{
		ErrProviderQuota:        model.HealthOutOfCredit,
		ErrProviderUnauthorized: model.HealthUnauthorized,
	} {
		clock := newClock()
		tr := newProviderTracker(clock.now)
		tr.observe(err)
		st := tr.current()
		if st.Health != want || !st.Since.Equal(clock.now()) || !st.RetryAfter.Equal(clock.now().Add(accountReprobe)) {
			t.Errorf("%v: status %+v, want %s with a 15 minute reprobe", err, st, want)
		}
		if _, ok := tr.admit(); ok {
			t.Errorf("%v: a call was admitted while the provider is blocked", err)
		}
	}
}

func TestServerFaultsTripOnlyAfterARunAndASuccessEndsTheRun(t *testing.T) {
	t.Parallel()
	tr := newProviderTracker(newClock().now)
	fault := fmt.Errorf("%w", ErrProviderUnavailable)
	tr.observe(fault)
	tr.observe(fault)
	tr.observe(nil)
	tr.observe(fault)
	tr.observe(fault)
	if h := tr.current().Health; h != model.HealthOK {
		t.Fatalf("health %s after two faults, a success and two more, want ok", h)
	}
	tr.observe(fault)
	if h := tr.current().Health; h != model.HealthDown {
		t.Errorf("health %s after three faults in a row, want down", h)
	}
}

func TestTimeoutsDegradeTheProviderWithoutBlockingIt(t *testing.T) {
	t.Parallel()
	tr := newProviderTracker(newClock().now)
	for range consecutiveToTrip {
		tr.observe(context.DeadlineExceeded)
	}
	st := tr.current()
	if st.Health != model.HealthDegraded || st.Health.Blocking() {
		t.Errorf("status %+v, want degraded and not blocking", st)
	}
	if _, ok := tr.admit(); !ok {
		t.Error("a degraded provider refused a call")
	}
	tr.observe(nil)
	if h := tr.current().Health; h != model.HealthOK {
		t.Errorf("health %s after a success, want ok", h)
	}
}

func TestAnUnreachableProviderIsDownAtOnce(t *testing.T) {
	t.Parallel()
	tr := newProviderTracker(newClock().now)
	tr.observe(&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED})
	if h := tr.current().Health; h != model.HealthDown {
		t.Errorf("health %s after one refused connection, want down", h)
	}
}

func TestAMessageTheProviderRefusedProvesItIsAnswering(t *testing.T) {
	t.Parallel()
	clock := newClock()
	tr := newProviderTracker(clock.now)
	tr.observe(ErrProviderQuota)
	clock.advance(accountReprobe)
	if _, ok := tr.admit(); !ok {
		t.Fatal("the probe was refused")
	}
	tr.observe(errors.New("ai: provider refused this request"))
	if h := tr.current().Health; h != model.HealthOK {
		t.Errorf("health %s after the provider answered a probe, want ok", h)
	}
}

func TestExactlyOneProbeIsAdmittedOnceTheRetryMomentPasses(t *testing.T) {
	t.Parallel()
	clock := newClock()
	tr := newProviderTracker(clock.now)
	tr.observe(ErrProviderUnauthorized)
	clock.advance(accountReprobe - time.Second)
	if _, ok := tr.admit(); ok {
		t.Fatal("a call was admitted before the retry moment")
	}
	clock.advance(time.Second)
	admitted := 0
	for range 5 {
		if _, ok := tr.admit(); ok {
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
	tr.observe(&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED})
	var waits []time.Duration
	for range 5 {
		st := tr.current()
		waits = append(waits, st.RetryAfter.Sub(clock.now()))
		clock.advance(st.RetryAfter.Sub(clock.now()))
		if _, ok := tr.admit(); !ok {
			t.Fatal("the probe was refused")
		}
		tr.observe(&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED})
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
	book.tracker("openai").observe(ErrProviderQuota)
	if len(book.snapshot()) != 1 {
		t.Fatal("the blocked provider is missing from the snapshot")
	}
	book.forget("openai")
	if got := book.snapshot(); len(got) != 0 {
		t.Errorf("snapshot %v after forgetting, want none", got)
	}
	if _, ok := book.tracker("openai").admit(); !ok {
		t.Error("a forgotten provider refused its next call")
	}
}

func TestTheSnapshotListsOnlyProvidersThatAreNotOKInNameOrder(t *testing.T) {
	t.Parallel()
	book := newProviderBook(newClock().now)
	book.tracker("openai").observe(ErrProviderQuota)
	book.tracker("anthropic").observe(ErrProviderUnauthorized)
	book.tracker("gemini").observe(nil)
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
	blocked.(*trackedClient).tracker.observe(ErrProviderUnauthorized)
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
