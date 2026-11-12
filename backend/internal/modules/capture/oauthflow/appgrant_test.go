// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package oauthflow

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestClientCredentialsPostsTheAppAndTheScopeAndReadsTheLifetime(t *testing.T) {
	var form url.Values
	srv := tokenServer(t, http.StatusOK, `{"access_token":"app-tok","expires_in":3599}`, &form)

	minted, err := New(testConfig(srv.URL, false)).ClientCredentials(context.Background(), "https://graph.microsoft.com/.default")
	if err != nil {
		t.Fatalf("ClientCredentials: %v", err)
	}
	if minted.AccessToken != "app-tok" || minted.ExpiresIn != 3599*time.Second {
		t.Errorf("minted = %+v, want app-tok for 3599s", minted)
	}
	for field, want := range map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     "cid",
		"client_secret": "secret",
		"scope":         "https://graph.microsoft.com/.default",
	} {
		if form.Get(field) != want {
			t.Errorf("form %s = %q, want %q", field, form.Get(field), want)
		}
	}
	if form.Has("refresh_token") || form.Has("code") {
		t.Errorf("an app grant carried a user grant's field: %v", form)
	}
}

func TestARefusedAppGrantIsAnAuthRejectionNamingTheMisconfiguredClient(t *testing.T) {
	for _, code := range []string{"invalid_client", "unauthorized_client"} {
		t.Run(code, func(t *testing.T) {
			var form url.Values
			srv := tokenServer(t, http.StatusUnauthorized, `{"error":"`+code+`"}`, &form)

			_, err := New(testConfig(srv.URL, false)).ClientCredentials(context.Background(), "s")
			if !errors.Is(err, errAuth) || errors.Is(err, errUnre) {
				t.Fatalf("err = %v, want the configured AuthRejected class", err)
			}
			if !Misconfigured(err) {
				t.Errorf("Misconfigured(%v) = false, want the deployment's app named as the fault", err)
			}
		})
	}
}

func TestAJWTBearerGrantPostsTheAssertionAlone(t *testing.T) {
	var form url.Values
	srv := tokenServer(t, http.StatusOK, `{"access_token":"sa-tok","expires_in":3600}`, &form)

	minted, err := New(testConfig(srv.URL, false)).JWTBearer(context.Background(), "a.b.c")
	if err != nil || minted.AccessToken != "sa-tok" {
		t.Fatalf("JWTBearer = %+v, %v", minted, err)
	}
	if form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || form.Get("assertion") != "a.b.c" {
		t.Errorf("form = %v, want the jwt-bearer grant with the assertion", form)
	}
	if form.Has("client_secret") {
		t.Error("the assertion grant carried a client secret it does not need")
	}
}

func TestAnAppGrantWithNoAccessTokenIsRejected(t *testing.T) {
	var form url.Values
	srv := tokenServer(t, http.StatusOK, `{"expires_in":3600}`, &form)

	if _, err := New(testConfig(srv.URL, false)).ClientCredentials(context.Background(), "s"); !errors.Is(err, errAuth) {
		t.Fatalf("err = %v, want AuthRejected", err)
	}
}

// fakeClock is a settable clock; nothing here reads the wall clock.
type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time { return c.at }

func countingMint(calls *int, lifetime time.Duration) func(context.Context) (MintedToken, error) {
	return func(context.Context) (MintedToken, error) {
		*calls++
		return MintedToken{AccessToken: "tok-" + strconv.Itoa(*calls), ExpiresIn: lifetime}, nil
	}
}

func TestACachedTokenIsReusedUntilFiveMinutesBeforeItExpires(t *testing.T) {
	clock := &fakeClock{at: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	cache := NewTokenCache(clock.now)
	calls := 0
	mint := countingMint(&calls, time.Hour)

	first, err := cache.Token(context.Background(), "k", mint)
	if err != nil {
		t.Fatal(err)
	}
	clock.at = clock.at.Add(54 * time.Minute)
	if again, err := cache.Token(context.Background(), "k", mint); err != nil || again != first || calls != 1 {
		t.Fatalf("at 54 minutes: token %q after %d mints (%v), want %q from one", again, calls, err, first)
	}
	clock.at = clock.at.Add(time.Minute)
	if next, err := cache.Token(context.Background(), "k", mint); err != nil || next == first || calls != 2 {
		t.Fatalf("at 55 minutes: token %q after %d mints (%v), want a second mint", next, calls, err)
	}
}

func TestATokenWithNoLifetimeIsNotKeptAndKeysDoNotShare(t *testing.T) {
	clock := &fakeClock{at: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	cache := NewTokenCache(clock.now)
	calls := 0

	for range 2 {
		if _, err := cache.Token(context.Background(), "k", countingMint(&calls, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Errorf("a token with no lifetime was minted %d times for two calls, want 2", calls)
	}
	if _, err := cache.Token(context.Background(), "other", countingMint(&calls, time.Hour)); err != nil || calls != 3 {
		t.Errorf("a second key reused the first key's token (%d mints, %v)", calls, err)
	}
	if CacheKey("tenant", "client", "secret-1") == CacheKey("tenant", "client", "secret-2") {
		t.Error("a rotated secret derives the same cache key")
	}
	if CacheKey("ab", "c") == CacheKey("a", "bc") {
		t.Error("two different part lists derive the same cache key")
	}
}

func TestAFailedMintIsNotCached(t *testing.T) {
	cache := NewTokenCache((&fakeClock{at: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}).now)
	refused := errors.New("refused")
	if _, err := cache.Token(context.Background(), "k", func(context.Context) (MintedToken, error) {
		return MintedToken{}, refused
	}); !errors.Is(err, refused) {
		t.Fatalf("err = %v, want the mint's own error", err)
	}
	calls := 0
	if _, err := cache.Token(context.Background(), "k", countingMint(&calls, time.Hour)); err != nil || calls != 1 {
		t.Errorf("after a failure the next call minted %d times (%v), want 1", calls, err)
	}
}

func TestTheCacheDropsExpiredTokensAndEvictsTheSoonestDueWhenFull(t *testing.T) {
	clock := &fakeClock{at: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	cache := NewTokenCache(clock.now)
	calls := 0
	if _, err := cache.Token(context.Background(), "short", countingMint(&calls, 10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	clock.at = clock.at.Add(6 * time.Minute)
	if _, err := cache.Token(context.Background(), "other", countingMint(&calls, 0)); err != nil {
		t.Fatal(err)
	}
	if _, kept := cache.tokens["short"]; kept {
		t.Error("a read left an expired token in the cache")
	}

	// Fill to the cap; the first key is due soonest, so it is the one evicted.
	for i := range maxCachedTokens + 1 {
		clock.at = clock.at.Add(time.Second)
		if _, err := cache.Token(context.Background(), "k"+strconv.Itoa(i), countingMint(&calls, time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if len(cache.tokens) != maxCachedTokens {
		t.Errorf("the cache holds %d tokens, want the cap %d", len(cache.tokens), maxCachedTokens)
	}
	if _, kept := cache.tokens["k0"]; kept {
		t.Error("the token due soonest survived a full cache")
	}
	if _, kept := cache.tokens["k"+strconv.Itoa(maxCachedTokens)]; !kept {
		t.Error("the newest token was not kept")
	}
}

func TestASharedMailboxAddressIsOnePlainAddress(t *testing.T) {
	if got, err := MailboxAddress("Team@Example.com"); err != nil || got != "team@example.com" {
		t.Fatalf("MailboxAddress = %q, %v; want the lowercased address", got, err)
	}
	for _, raw := range []string{
		"", "team", "Team <team@example.com>", "a@example.com, b@example.com",
		"team/../x@example.com", `team\x@example.com`, "team%40@example.com", "team?@example.com",
		"team#@example.com", "team @example.com", "team@example.com\n", "team\x00@example.com",
	} {
		if _, err := MailboxAddress(raw); !errors.Is(err, ErrMailboxAddress) {
			t.Errorf("MailboxAddress(%q) = %v, want ErrMailboxAddress", raw, err)
		}
	}
}
