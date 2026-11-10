// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestGmailWeightsShareLiveHistoryAndCanonicalMailboxIdentity(t *testing.T) {
	now := time.Unix(0, 0)
	registry := ratelimit.NewRegistry()
	var waits []time.Duration
	quota := newQuotaPacing(6100, 1200100)
	quota.now = func() time.Time { return now }
	quota.meter = registry.NewPacer(func() time.Time { return now })
	quota.sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); now = now.Add(d); return nil }
	live, history := &httpAPI{quota: quota}, &httpAPI{quota: quota}
	if err := live.waitQuota(quotaOwner(t.Context(), "Owner@Example.com"), 20); err != nil {
		t.Fatal(err)
	}
	if err := history.waitQuota(quotaOwner(t.Context(), "owner@example.com"), 60); err != nil {
		t.Fatal(err)
	}
	if len(waits) != 1 || waits[0] != 200*time.Millisecond {
		t.Fatalf("live/history had separate mailbox quota: %v", waits)
	}
	if err := live.waitQuota(quotaOwner(t.Context(), "owner@example.com"), 20); err != nil {
		t.Fatal(err)
	}
	if len(waits) != 2 || waits[1] != 600*time.Millisecond {
		t.Fatalf("raw request was not weighted: %v", waits)
	}
}

func TestGmailProjectPacingSpansDifferentMailboxes(t *testing.T) {
	now := time.Unix(0, 0)
	quota := newQuotaPacing(1000000, 6100)
	quota.now = func() time.Time { return now }
	quota.meter = ratelimit.NewRegistry().NewPacer(func() time.Time { return now })
	var waited time.Duration
	quota.sleep = func(_ context.Context, d time.Duration) error { waited += d; now = now.Add(d); return nil }
	api := &httpAPI{quota: quota}
	for _, owner := range []string{"first@example.com", "second@example.com"} {
		if err := api.waitQuota(quotaOwner(t.Context(), owner), 60); err != nil {
			t.Fatal(err)
		}
	}
	if waited != 600*time.Millisecond {
		t.Fatalf("project usage split by mailbox: %v", waited)
	}
}

func TestGmailQuotaWeightsMatchRESTFormats(t *testing.T) {
	for _, test := range []struct {
		path, format string
		want         int
	}{{"/profile", "", 1}, {"/labels", "", 1}, {"/history", "", 2}, {"/messages", "", 5}, {"/messages/id", "metadata", 20}, {"/messages/id", "", 60}, {"/messages/id", "RAW", 60}, {"/messages/id", "raw", 60}, {"/messages/id", "full", 60}} {
		if got := getQuotaCost(test.path, url.Values{"format": {test.format}}); got != test.want {
			t.Fatalf("%s/%s = %d want %d", test.path, test.format, got, test.want)
		}
	}
}

func newTestAPI(t testing.TB, client *http.Client, base string) API {
	t.Helper()
	value := NewAPI(client, base)
	api, ok := value.(*httpAPI)
	if !ok {
		t.Fatal("production Gmail HTTP client missing")
		return nil
	}
	clock := &quotaHTTP{now: time.Unix(0, 0)}
	api.quota.now = clock.clock
	api.quota.meter = ratelimit.NewRegistry().NewPacer(clock.clock)
	api.quota.sleep = clock.advance
	return api
}

func TestGmailProviderCooldownKeepsItsScopeAndYieldsLongWaits(t *testing.T) {
	for _, mailboxScoped := range []bool{true, false} {
		now := time.Unix(0, 0)
		quota := newQuotaPacing(0, 0)
		quota.now = func() time.Time { return now }
		quota.meter = ratelimit.NewRegistry().NewPacer(func() time.Time { return now })
		quota.sleep = func(context.Context, time.Duration) error {
			t.Fatal("long cooldown slept while holding a page")
			return nil
		}
		first, second := &httpAPI{quota: quota}, &httpAPI{quota: quota}
		ctx := quotaOwner(t.Context(), "first@example.com")
		reason := connector.RateLimitQuota
		if mailboxScoped {
			reason = connector.RateLimitUser
		}
		cause := &connector.RateLimitedError{Reason: reason, RetryAfter: 2 * time.Minute}
		if err := first.noteQuotaLimit(ctx, cause); !errors.Is(err, connector.ErrRateLimited) {
			t.Fatal(err)
		}
		err := second.waitQuota(ctx, 60)
		limited, ok := errors.AsType[*connector.RateLimitedError](err)
		if !ok || limited.RetryAfter != 2*time.Minute || limited.MailboxScoped() != mailboxScoped {
			t.Fatalf("scope changed: %v", err)
		}
		otherErr := second.waitQuota(quotaOwner(t.Context(), "other@example.com"), 60)
		if mailboxScoped && otherErr != nil || !mailboxScoped && !errors.Is(otherErr, connector.ErrRateLimited) {
			t.Fatalf("another mailbox got wrong cooldown: %v", otherErr)
		}
	}
}

func TestGmailListingYieldsAThirtySecondMailboxPause(t *testing.T) {
	apiValue := NewAPI(&http.Client{Transport: &quotaHTTP{}}, "https://gmail.test")
	api, ok := apiValue.(*httpAPI)
	if !ok {
		t.Fatal("production client missing")
	}
	quota := newQuotaPacing(0, 0)
	now := time.Unix(0, 0)
	quota.now = func() time.Time { return now }
	quota.meter = ratelimit.NewRegistry().NewPacer(quota.now)
	quota.sleep = func(context.Context, time.Duration) error {
		t.Fatal("listing held its execution during a long pause")
		return nil
	}
	api.quota = quota
	cause := &connector.RateLimitedError{Reason: connector.RateLimitUser, RetryAfter: 30 * time.Second}
	if err := api.noteQuotaLimit(quotaOwner(t.Context(), owner), cause); !errors.Is(err, connector.ErrRateLimited) {
		t.Fatal(err)
	}
	_, err := New(fakeOAuth{access: "access"}, api).BackfillPage(t.Context(), authBytes(t), now, "", &recordingSink{})
	limited, ok := errors.AsType[*connector.RateLimitedError](err)
	if !ok || limited.RetryAfter != 30*time.Second || !limited.MailboxScoped() {
		t.Fatalf("listing did not yield scoped wait: %v", err)
	}
}

func TestGmailUnknownOwnerDoesNotPauseOtherMailboxes(t *testing.T) {
	quota := newQuotaPacing(0, 0)
	now := time.Unix(0, 0)
	quota.now = func() time.Time { return now }
	quota.meter = ratelimit.NewRegistry().NewPacer(quota.now)
	api := &httpAPI{quota: quota}
	cause := &connector.RateLimitedError{Reason: connector.RateLimitUser, RetryAfter: time.Hour}
	if err := api.noteQuotaLimit(t.Context(), cause); !errors.Is(err, connector.ErrRateLimited) {
		t.Fatal(err)
	}
	if err := api.waitQuota(quotaOwner(t.Context(), "unrelated@example.com"), 60); err != nil {
		t.Fatalf("unknown owner's limit stopped everyone: %v", err)
	}
}

func TestGmailListingCannotRestartTheCumulativePacingBudget(t *testing.T) {
	now := time.Unix(0, 0)
	quota := newQuotaPacing(6100, 1200100)
	quota.now = func() time.Time { return now }
	quota.meter = ratelimit.NewRegistry().NewPacer(quota.now)
	ctx := quotaOwner(t.Context(), owner)
	budget := ratelimit.Budget{Key: mailboxQuotaKey(owner), PerMinute: 6000, Cost: 60}
	if wait, _, err := quota.meter.Reserve(ctx, budget); wait != 0 || err != nil {
		t.Fatal(err)
	}
	waits := 0
	quota.sleep = func(ctx context.Context, d time.Duration) error {
		waits++
		now = now.Add(d)
		if wait, _, err := quota.meter.Reserve(ctx, budget); wait != 0 || err != nil {
			t.Fatalf("competing request: %v %v", wait, err)
		}
		return nil
	}
	api := &httpAPI{quota: quota}
	_, err := (&Connector{api: api}).listHistoryPage(ctx, "access", "after:2026/01/01", "", &rateGate{})
	if !errors.Is(err, connector.ErrRateLimited) || waits != 8 || now.Sub(time.Unix(0, 0)) > rateWaitInPage {
		t.Fatalf("listing restarted pacing wait: sleeps=%d elapsed=%v err=%v", waits, now.Sub(time.Unix(0, 0)), err)
	}
}

type unavailableQuotaCoordinator struct{}

func (unavailableQuotaCoordinator) Reserve(context.Context, ...ratelimit.Budget) (time.Duration, string, error) {
	return 0, "", errors.New("quota store unavailable")
}

func (unavailableQuotaCoordinator) Pause(context.Context, string, time.Duration) error {
	return errors.New("quota store unavailable")
}

func TestGmailQuotaStoreOutageMakesNoHTTPCall(t *testing.T) {
	provider := &quotaHTTP{}
	value := NewAPI(&http.Client{Transport: provider}, "https://gmail.test")
	api, ok := value.(*httpAPI)
	if !ok {
		t.Fatal("production client missing")
	}
	api.quota.meter = unavailableQuotaCoordinator{}
	if _, _, err := api.Profile(t.Context(), "access"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("outage admitted HTTP: %v", err)
	}
	cause := &connector.RateLimitedError{Reason: connector.RateLimitQuota, RetryAfter: time.Minute}
	if err := api.noteQuotaLimit(t.Context(), cause); !errors.Is(err, ErrUnreachable) || !errors.Is(err, connector.ErrRateLimited) {
		t.Fatalf("pause outage lost original limit: %v", err)
	}
	if provider.requests != 0 {
		t.Fatalf("outage made %d HTTP calls", provider.requests)
	}
}

type rotatingQuotaOAuth struct {
	fakeOAuth
	calls int
}

func (a *rotatingQuotaOAuth) AccessToken(ctx context.Context, _ string) (string, error) {
	owner, ok := ctx.Value(quotaOwnerKey{}).(string)
	if !ok || owner != "owner@example.com" {
		return "", errors.New("mailbox missing before token refresh")
	}
	a.calls++
	if a.calls == 1 {
		return "first-access", nil
	}
	return "second-access", nil
}

func TestAuthenticatedGmailRefreshKeepsOneQuotaAcrossCredentialRotation(t *testing.T) {
	oauth := &rotatingQuotaOAuth{}
	c := New(oauth, &fakeAPI{})
	first, firstAccess, err := c.authorize(t.Context(), authState{Owner: " Owner@Example.com ", RefreshToken: "old-refresh"})
	if err != nil {
		t.Fatal(err)
	}
	second, secondAccess, err := c.authorize(t.Context(), authState{Owner: "owner@example.com", RefreshToken: "new-refresh"})
	if err != nil || firstAccess == secondAccess {
		t.Fatalf("rotation failed: %v", err)
	}
	now := time.Unix(0, 0)
	quota := newQuotaPacing(6100, 1200100)
	quota.now = func() time.Time { return now }
	quota.meter = ratelimit.NewRegistry().NewPacer(quota.now)
	var waited time.Duration
	quota.sleep = func(_ context.Context, d time.Duration) error { waited += d; now = now.Add(d); return nil }
	api := &httpAPI{quota: quota}
	if err := api.waitQuota(first, 60); err != nil {
		t.Fatal(err)
	}
	if err := api.waitQuota(second, 20); err != nil {
		t.Fatal(err)
	}
	if waited != 600*time.Millisecond {
		t.Fatalf("credential rotation opened a new quota: %v", waited)
	}
}
