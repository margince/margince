// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package oauthflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// countingTokenEndpoint answers every refresh with a one-hour token. With
// rotate set it also issues a new refresh token on every redemption.
func countingTokenEndpoint(t *testing.T, rotate bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		refresh := r.PostForm.Get("refresh_token")
		if rotate {
			refresh = fmt.Sprintf("rt-%d", n)
		}
		fmt.Fprintf(w, `{"access_token":"at-%d","refresh_token":%q,"expires_in":3600}`, n, refresh)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func cachingClient(tokenURL string, clock *fakeClock) *Client {
	cfg := testConfig(tokenURL, true)
	cfg.Tokens = NewTokenCache(clock.now, 8)
	return New(cfg)
}

func TestRefreshInsideTheLifetimeAsksTheProviderOnce(t *testing.T) {
	srv, calls := countingTokenEndpoint(t, false)
	clock := &fakeClock{at: time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)}
	c := cachingClient(srv.URL, clock)
	first, err := c.Refresh(context.Background(), "rt", baseScope)
	if err != nil {
		t.Fatal(err)
	}
	clock.at = clock.at.Add(50 * time.Minute)
	second, err := c.Refresh(context.Background(), "rt", baseScope)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || second.AccessToken != first.AccessToken || second.Rotated != "" {
		t.Fatalf("two refreshes in one lifetime: %d requests, tokens %q then %q, rotated %q; want 1 request and one token",
			calls.Load(), first.AccessToken, second.AccessToken, second.Rotated)
	}
	clock.at = clock.at.Add(6 * time.Minute)
	if _, err := c.Refresh(context.Background(), "rt", baseScope); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("a refresh past the token's replacement time made %d requests in all, want 2", calls.Load())
	}
}

func TestARotatedTokenFindsTheTokenItsRedemptionMinted(t *testing.T) {
	srv, calls := countingTokenEndpoint(t, true)
	clock := &fakeClock{at: time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)}
	c := cachingClient(srv.URL, clock)
	first, err := c.Refresh(context.Background(), "rt-0", baseScope)
	if err != nil {
		t.Fatal(err)
	}
	if first.Rotated != "rt-1" {
		t.Fatalf("a miss reported rotation %q, want rt-1 for the caller to store", first.Rotated)
	}
	second, err := c.Refresh(context.Background(), first.Rotated, baseScope)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || second.AccessToken != first.AccessToken || second.Rotated != "" {
		t.Fatalf("second sync on the stored rotation: %d requests, rotated %q; want 1 request and no rotation",
			calls.Load(), second.Rotated)
	}
}

func TestTwoScopeListsDoNotShareAToken(t *testing.T) {
	srv, calls := countingTokenEndpoint(t, false)
	c := cachingClient(srv.URL, &fakeClock{at: time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)})
	granted, err := c.Refresh(context.Background(), "rt", baseScope)
	if err != nil {
		t.Fatal(err)
	}
	unscoped, err := c.AccessToken(context.Background(), "rt")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || unscoped == granted.AccessToken {
		t.Fatalf("two scope lists: %d requests, tokens %q and %q; want two requests and two tokens",
			calls.Load(), granted.AccessToken, unscoped)
	}
}

func TestRenewingRedeemsPastAKeptTokenAndReplacesIt(t *testing.T) {
	srv, calls := countingTokenEndpoint(t, false)
	c := cachingClient(srv.URL, &fakeClock{at: time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)})
	if _, err := c.Refresh(context.Background(), "rt", baseScope); err != nil {
		t.Fatal(err)
	}
	renewed, err := c.Refresh(Renewing(context.Background()), "rt", baseScope)
	if err != nil {
		t.Fatal(err)
	}
	after, err := c.Refresh(context.Background(), "rt", baseScope)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || after.AccessToken != renewed.AccessToken {
		t.Fatalf("renewal: %d requests, then token %q after renewing to %q; want 2 requests and the renewed token kept",
			calls.Load(), after.AccessToken, renewed.AccessToken)
	}
}
