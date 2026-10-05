// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// ErrProviderUnauthorized marks a provider refusing the credential (401/403).
// Like ErrProviderQuota it is the operator's to fix; no retry changes it.
var ErrProviderUnauthorized = errors.New("ai: the configured AI provider rejected the credential")

// ErrProviderUnavailable marks a provider answering with a server fault (5xx).
var ErrProviderUnavailable = errors.New("ai: the configured AI provider is unavailable")

// ErrProviderDown is returned, with no call made and nothing charged, when the
// provider behind every bound rung is known to be blocked. It is a deferral
// like ErrBudgetDeferred: the work waits for ProviderDownError.RetryAfter.
var ErrProviderDown = errors.New("ai: the AI provider is down")

// ProviderDownError says which provider blocked the call, why, and when one
// probe is next allowed.
type ProviderDownError struct {
	Provider   string
	Health     model.ProviderHealth
	RetryAfter time.Time
}

func (e *ProviderDownError) Error() string {
	return fmt.Sprintf("ai: provider %s is %s until %s", e.Provider, e.Health, e.RetryAfter.Format(time.RFC3339))
}

func (e *ProviderDownError) Unwrap() error { return ErrProviderDown }

// IsDeferral reports whether err means "not now, and not this item's fault":
// the installation's budget stop or a blocked provider. The attempt is refunded
// and the pass stops.
func IsDeferral(err error) bool {
	return errors.Is(err, ErrBudgetDeferred) || errors.Is(err, ErrProviderDown)
}

// DeferredUntil is the moment a deferred item may be tried again.
func DeferredUntil(err error) (time.Time, bool) {
	var budget *BudgetDeferralError
	if errors.As(err, &budget) {
		return budget.NextAttemptAt, true
	}
	var down *ProviderDownError
	if errors.As(err, &down) {
		return down.RetryAfter, true
	}
	return time.Time{}, false
}

const (
	// consecutiveToTrip is how many 5xx or timeouts in a row make a provider
	// down or degraded; one slow message must not.
	consecutiveToTrip = 3
	// accountReprobe is how often an out-of-credit or unauthorized provider is
	// probed. A rebind probes at once (forget).
	accountReprobe = 15 * time.Minute
	downProbeFloor = 30 * time.Second
	downProbeCap   = 5 * time.Minute
)

type failureKind int

const (
	failNone failureKind = iota
	failCredit
	failAuth
	failUnreachable
	failServer
	failTimeout
)

// classifyFailure says whether err is the provider failing for everyone. Any
// other error, including a message the provider refused, means it answered.
func classifyFailure(err error) failureKind {
	var dns *net.DNSError
	var op *net.OpError
	switch {
	case err == nil, errors.Is(err, context.Canceled):
		return failNone
	case errors.Is(err, ErrProviderQuota):
		return failCredit
	case errors.Is(err, ErrProviderUnauthorized):
		return failAuth
	case errors.Is(err, ErrProviderUnavailable):
		return failServer
	case errors.As(err, &dns), errors.Is(err, syscall.ECONNREFUSED),
		errors.As(err, &op) && op.Op == "dial":
		return failUnreachable
	case errors.Is(err, context.DeadlineExceeded):
		return failTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return failTimeout
	}
	return failNone
}

// providerTracker is one provider's health. Every tier bound to the provider
// shares it: one empty account is one fact.
type providerTracker struct {
	mu          sync.Mutex
	now         func() time.Time
	status      model.ProviderHealthStatus
	consecutive int
	probing     bool
	backoff     time.Duration
}

func newProviderTracker(now func() time.Time) *providerTracker {
	return &providerTracker{now: now, status: model.ProviderHealthStatus{Health: model.HealthOK}}
}

func (t *providerTracker) current() model.ProviderHealthStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// admit lets a call through, or refuses it with the blocking status. Once the
// retry moment passes, exactly one caller is let through as the probe.
func (t *providerTracker) admit() (model.ProviderHealthStatus, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.status.Health.Blocking() {
		return t.status, true
	}
	if t.now().Before(t.status.RetryAfter) || t.probing {
		return t.status, false
	}
	t.probing = true
	return t.status, true
}

// observe records one call's outcome.
func (t *providerTracker) observe(err error) {
	kind := classifyFailure(err)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.probing = false
	if kind == failNone {
		if errors.Is(err, context.Canceled) {
			return
		}
		t.recover()
		return
	}
	t.fail(kind)
}

func (t *providerTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.probing = false
	t.recover()
}

func (t *providerTracker) recover() {
	t.status = model.ProviderHealthStatus{Health: model.HealthOK}
	t.consecutive, t.backoff = 0, 0
}

func (t *providerTracker) fail(kind failureKind) {
	now := t.now()
	switch kind {
	case failCredit:
		t.trip(model.HealthOutOfCredit, now, accountReprobe)
	case failAuth:
		t.trip(model.HealthUnauthorized, now, accountReprobe)
	case failUnreachable:
		t.trip(model.HealthDown, now, t.nextBackoff())
	case failServer, failTimeout:
		t.consecutive++
		switch {
		case t.status.Health.Blocking():
			t.trip(model.HealthDown, now, t.nextBackoff())
		case t.consecutive < consecutiveToTrip:
		case kind == failServer:
			t.trip(model.HealthDown, now, t.nextBackoff())
		default:
			t.status = model.ProviderHealthStatus{Health: model.HealthDegraded, Since: t.since(now)}
		}
	}
}

func (t *providerTracker) trip(h model.ProviderHealth, now time.Time, wait time.Duration) {
	t.status = model.ProviderHealthStatus{Health: h, Since: t.since(now), RetryAfter: now.Add(wait)}
}

func (t *providerTracker) since(now time.Time) time.Time {
	if t.status.Health != model.HealthOK && !t.status.Since.IsZero() {
		return t.status.Since
	}
	return now
}

func (t *providerTracker) nextBackoff() time.Duration {
	t.backoff = min(max(t.backoff*2, downProbeFloor), downProbeCap)
	return t.backoff
}

// providerBook holds one tracker per provider name for the process. It is
// shared because several Routers are minted over one routing config, and they
// must agree on whether a provider is up.
type providerBook struct {
	mu       sync.Mutex
	now      func() time.Time
	trackers map[string]*providerTracker
}

func newProviderBook(now func() time.Time) *providerBook {
	return &providerBook{now: now, trackers: map[string]*providerTracker{}}
}

var sharedProviderHealth = newProviderBook(time.Now)

func (b *providerBook) tracker(provider string) *providerTracker {
	b.mu.Lock()
	defer b.mu.Unlock()
	t, ok := b.trackers[provider]
	if !ok {
		t = newProviderTracker(b.now)
		b.trackers[provider] = t
	}
	return t
}

// forget clears a provider's recorded failures in place, so every client
// already holding its tracker sees it: a saved key or a rebind is the
// operator's fix, and the next call is the probe.
func (b *providerBook) forget(provider string) {
	b.mu.Lock()
	t, ok := b.trackers[provider]
	b.mu.Unlock()
	if ok {
		t.reset()
	}
}

// ProviderHealthEntry is one provider's health as the operator surfaces read it.
type ProviderHealthEntry struct {
	Provider string
	Status   model.ProviderHealthStatus
}

// snapshot lists every provider that is not OK, by name.
func (b *providerBook) snapshot() []ProviderHealthEntry {
	b.mu.Lock()
	names := make([]string, 0, len(b.trackers))
	for name := range b.trackers {
		names = append(names, name)
	}
	b.mu.Unlock()
	sort.Strings(names)
	var out []ProviderHealthEntry
	for _, name := range names {
		if st := b.tracker(name).current(); st.Health != model.HealthOK {
			out = append(out, ProviderHealthEntry{Provider: name, Status: st})
		}
	}
	return out
}

// trackedClient is a provider's client reporting into its tracker. Selection
// wraps every client in one, so no adapter observes its own calls.
type trackedClient struct {
	model.Client
	provider string
	tracker  *providerTracker
}

func trackClient(c model.Client, provider string, book *providerBook) model.Client {
	return &trackedClient{Client: c, provider: provider, tracker: book.tracker(provider)}
}

func (c *trackedClient) admit() error {
	if st, ok := c.tracker.admit(); !ok {
		return &ProviderDownError{Provider: c.provider, Health: st.Health, RetryAfter: st.RetryAfter}
	}
	return nil
}

func (c *trackedClient) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	if err := c.admit(); err != nil {
		return model.Response{}, err
	}
	resp, err := c.Client.Complete(ctx, req)
	c.tracker.observe(err)
	return resp, err
}

func (c *trackedClient) Stream(ctx context.Context, req model.Request) (model.TokenStream, error) {
	if err := c.admit(); err != nil {
		return nil, err
	}
	stream, err := c.Client.Stream(ctx, req)
	c.tracker.observe(err)
	return stream, err
}

func (c *trackedClient) Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	if err := c.admit(); err != nil {
		return model.Embeddings{}, err
	}
	out, err := c.Client.Embed(ctx, req)
	c.tracker.observe(err)
	return out, err
}

func (c *trackedClient) Health() model.ProviderHealthStatus { return c.tracker.current() }

// blockedProvider is the error for a call whose every bound rung sits on a
// blocked provider, nil when any rung may be tried. The earliest retry moment
// wins, so a deferred item wakes when the first provider may answer.
func blockedProvider(b *binding, ladder []Tier, now time.Time) *ProviderDownError {
	var first *ProviderDownError
	for _, t := range ladder {
		client, ok := b.clients[t]
		if !ok {
			continue
		}
		st := client.Health()
		if !st.Blocked(now) {
			return nil
		}
		if first == nil || st.RetryAfter.Before(first.RetryAfter) {
			first = &ProviderDownError{Provider: b.routeMeta[t].provider, Health: st.Health, RetryAfter: st.RetryAfter}
		}
	}
	return first
}

// ProviderHealth lists every provider that is not answering normally.
func (r *Router) ProviderHealth() []ProviderHealthEntry { return sharedProviderHealth.snapshot() }
