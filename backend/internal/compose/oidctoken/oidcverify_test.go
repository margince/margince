// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package oidctoken

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/oidctoken/oidctokentest"
)

// newTestVerifier pins the verifier's clock to the rig's fixed base instant;
// tests that exercise time behavior override it via SetClock.
func testMatchIdentity(c Claims) error {
	if !c.Aud.Is(oidctokentest.Aud) {
		return fmt.Errorf("%w: aud mismatch", ErrRejected)
	}
	if c.Email != oidctokentest.SA {
		return fmt.Errorf("%w: email mismatch", ErrRejected)
	}
	if !c.EmailVerified {
		return fmt.Errorf("%w: email not verified", ErrRejected)
	}
	return nil
}

func newTestVerifier(rig *oidctokentest.Rig) *Verifier {
	return NewGoogle(rig.JWKSURL(), testMatchIdentity).
		WithHTTPClient(rig.Srv.Client()).
		SetClock(func() time.Time { return rig.Base })
}

func TestOIDCVerifyAcceptsValidToken(t *testing.T) {
	rig := oidctokentest.NewRig(t)
	tok := rig.Mint(t, oidctokentest.KID, "RS256", nil)
	if _, err := newTestVerifier(rig).Verify(context.Background(), tok); err != nil {
		t.Fatalf("Verify(valid) = %v, want nil", err)
	}
}

func TestOIDCVerifyReturnsClaims(t *testing.T) {
	rig := oidctokentest.NewRig(t)
	tok := rig.Mint(t, oidctokentest.KID, "RS256", map[string]any{"sub": "1234567890"})
	claims, err := newTestVerifier(rig).Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Sub != "1234567890" {
		t.Fatalf("Sub = %q, want 1234567890", claims.Sub)
	}
}

func TestOIDCVerifyRejects(t *testing.T) {
	rig := oidctokentest.NewRig(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		tok  func() string
	}{
		{"empty", func() string { return "" }},
		{"not-three-segments", func() string { return "a.b" }},
		{"alg-none", func() string { return rig.Mint(t, oidctokentest.KID, "none", nil) }},
		{"unknown-kid", func() string { return rig.Mint(t, "nope", "RS256", nil) }},
		{"wrong-aud", func() string {
			return rig.Mint(t, oidctokentest.KID, "RS256", map[string]any{"aud": "https://evil.test"})
		}},
		{"wrong-email", func() string {
			return rig.Mint(t, oidctokentest.KID, "RS256", map[string]any{"email": "attacker@evil.test"})
		}},
		{"email-unverified", func() string { return rig.Mint(t, oidctokentest.KID, "RS256", map[string]any{"email_verified": false}) }},
		{"wrong-iss", func() string {
			return rig.Mint(t, oidctokentest.KID, "RS256", map[string]any{"iss": "https://evil.test"})
		}},
		{"expired", func() string {
			return rig.Mint(t, oidctokentest.KID, "RS256", map[string]any{"exp": rig.Base.Add(-time.Hour).Unix()})
		}},
		{"future-iat", func() string {
			return rig.Mint(t, oidctokentest.KID, "RS256", map[string]any{"iat": rig.Base.Add(time.Hour).Unix()})
		}},
		{"missing-iat", func() string {
			return rig.Mint(t, oidctokentest.KID, "RS256", map[string]any{"iat": 0})
		}},
		{"bad-signature", func() string {
			// A token signed by a DIFFERENT key but advertising the served kid.
			return rig.SignedWith(other).Mint(t, oidctokentest.KID, "RS256", nil)
		}},
	}
	v := newTestVerifier(rig)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := v.Verify(context.Background(), tc.tok()); err == nil {
				t.Fatalf("Verify(%s) = nil, want an error", tc.name)
			}
		})
	}
}

// TestOIDCVerifyHonorsInjectedClock exercises the SetClock test seam. A token
// accepted "now" is rejected once the clock moves past exp+skew, and again when
// it moves before iat-skew.
func TestOIDCVerifyHonorsInjectedClock(t *testing.T) {
	rig := oidctokentest.NewRig(t)
	iat := rig.Base
	exp := iat.Add(time.Hour)
	tok := rig.Mint(t, oidctokentest.KID, "RS256", map[string]any{
		"iat": iat.Unix(),
		"exp": exp.Unix(),
	})

	atIssue := newTestVerifier(rig).SetClock(func() time.Time { return iat })
	if _, err := atIssue.Verify(context.Background(), tok); err != nil {
		t.Fatalf("Verify(at issue) = %v, want nil", err)
	}

	longAfterExpiry := newTestVerifier(rig).SetClock(func() time.Time { return exp.Add(time.Hour) })
	if _, err := longAfterExpiry.Verify(context.Background(), tok); err == nil {
		t.Fatal("Verify(long after exp) = nil, want an error")
	}

	longBeforeIssue := newTestVerifier(rig).SetClock(func() time.Time { return iat.Add(-time.Hour) })
	if _, err := longBeforeIssue.Verify(context.Background(), tok); err == nil {
		t.Fatal("Verify(long before iat) = nil, want an error")
	}
}

// TestOIDCVerifyThrottlesJWKSRefresh proves the cross-call refresh bound. An
// unauthenticated caller forces a cache miss with a never-seen kid, and needs
// no valid signature to reach the key lookup. So refreshes are throttled
// however many distinct kids arrive within the cooldown.
func TestOIDCVerifyThrottlesJWKSRefresh(t *testing.T) {
	rig := oidctokentest.NewRig(t)
	now := rig.Base
	v := newTestVerifier(rig).SetClock(func() time.Time { return now })

	tok1 := rig.Mint(t, "unknown-kid-1", "RS256", nil)
	tok2 := rig.Mint(t, "unknown-kid-2", "RS256", nil)

	if _, err := v.Verify(context.Background(), tok1); err == nil {
		t.Fatal("Verify(unknown kid 1) = nil, want an error")
	}
	if _, err := v.Verify(context.Background(), tok2); err == nil {
		t.Fatal("Verify(unknown kid 2) = nil, want an error")
	}
	if got := rig.CertsHitCount(); got != 1 {
		t.Fatalf("certs hits after two distinct unknown kids within cooldown = %d, want 1", got)
	}

	// Advance the injected clock past the cooldown: the next unknown-kid
	// attempt must trigger a second fetch.
	now = now.Add(jwksRefreshCooldown)
	tok3 := rig.Mint(t, "unknown-kid-3", "RS256", nil)
	if _, err := v.Verify(context.Background(), tok3); err == nil {
		t.Fatal("Verify(unknown kid 3) = nil, want an error")
	}
	if got := rig.CertsHitCount(); got != 2 {
		t.Fatalf("certs hits after cooldown elapsed = %d, want 2", got)
	}

	// The accept path for a real, cached kid still works after all this.
	valid := rig.Mint(t, oidctokentest.KID, "RS256", nil)
	if _, err := v.Verify(context.Background(), valid); err != nil {
		t.Fatalf("Verify(valid) = %v, want nil", err)
	}
}

// TestOIDCVerifyCoalescesConcurrentJWKSRefresh proves a cold-cache burst
// shares one JWKS fetch instead of the cooldown rejecting valid requests.
// While the first fetch is held in flight, a second valid caller waits for its
// result and succeeds: one certs hit, no 401.
func TestOIDCVerifyCoalescesConcurrentJWKSRefresh(t *testing.T) {
	rig := oidctokentest.NewRig(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	rig.CertsGate = func() {
		once.Do(func() { close(entered) })
		<-release
	}

	v := newTestVerifier(rig)
	tok := rig.Mint(t, oidctokentest.KID, "RS256", nil)

	errA := make(chan error, 1)
	go func() { _, err := v.Verify(context.Background(), tok); errA <- err }()
	// The first refresh is now registered and its fetch is held in flight.
	<-entered
	// Released only once B is waiting on A's fetch. Racing B's arrival let A
	// finish first, and B then met a cold cache: a second fetch or the
	// cooldown's refusal. That read as a failure to coalesce it was never asked
	// to make.
	joined := make(chan struct{})
	v.coalesced = func() { close(joined) }
	errB := make(chan error, 1)
	go func() { _, err := v.Verify(context.Background(), tok); errB <- err }()
	// Bounded: a verifier that stopped coalescing never signals, and an
	// unbounded receive hangs the package instead of naming the defect. This is
	// not a timing assumption, because the signal precedes any I/O.
	select {
	case <-joined:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("the second caller never joined the in-flight refresh: it started its own fetch " +
			"or met the cooldown, which is the coalescing this test exists to prove")
	}
	close(release)

	if err := <-errA; err != nil {
		t.Fatalf("Verify(first, cold cache) = %v, want nil", err)
	}
	if err := <-errB; err != nil {
		t.Fatalf("Verify(concurrent with in-flight refresh) = %v, want nil (coalesced, not throttled)", err)
	}
	if got := rig.CertsHitCount(); got != 1 {
		t.Fatalf("certs hits after concurrent cold-cache burst = %d, want 1", got)
	}
}

func TestAnAudienceIsReadAsAStringOrAListAndMustNameOnlyUs(t *testing.T) {
	for raw, want := range map[string]bool{
		`{"aud":"app:x"}`:           true,
		`{"aud":["app:x"]}`:         true,
		`{"aud":["app:x","app:y"]}`: false,
		`{"aud":["app:y"]}`:         false,
		`{"aud":[]}`:                false,
		`{}`:                        false,
	} {
		var c Claims
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if got := c.Aud.Is("app:x"); got != want {
			t.Errorf("%s: Is(app:x) = %t, want %t", raw, got, want)
		}
	}
	var c Claims
	if err := json.Unmarshal([]byte(`{"aud":7}`), &c); err == nil {
		t.Error("a numeric aud was read")
	}
}
