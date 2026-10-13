// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// The installation's access-token lifetime
// (installation.oauth_access_token_ttl_minutes). A connector's access token is a
// passport, and a passport defaults to 30 days where connector norms are
// minutes plus refresh — this is the setting that closes that gap, and it has
// to reach BOTH mints of a connection's life, because a 15-minute token that
// every rotation re-issues for 30 days is not a 15-minute token.

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// accessTokenLifetime reads the RFC 6749 §5.1 expires_in a token response
// carries — the number a client actually schedules its renewal from.
func accessTokenLifetime(t *testing.T, body map[string]any) time.Duration {
	t.Helper()
	seconds, ok := body["expires_in"].(float64)
	if !ok {
		t.Fatalf("token response carries no expires_in: %v", body)
	}
	return time.Duration(seconds) * time.Second
}

// setAccessTokenTTL changes the lifetime the way an admin does, through the
// Settings surface, so the mint reads it exactly as it reads any change.
func setAccessTokenTTL(t *testing.T, o *oauthEnv, ttl time.Duration) {
	t.Helper()
	if status := o.Call(t, "PATCH", "/v1/installation/settings", integration.AnyMap{
		"oauth_access_token_ttl_minutes": int(ttl / time.Minute),
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("setting the access-token lifetime → %d", status)
	}
}

// withinAMinuteOf allows the second or two the mint and the response take,
// with a lower bound too: a lifetime the mint silently dropped would read as
// 30 days, and one mangled to zero as an already-expired token.
func withinAMinuteOf(got, want time.Duration) bool {
	return got <= want && got >= want-time.Minute
}

// A chosen lifetime shortens the code exchange's passport AND every rotation's.
func TestAChosenAccessTokenTTLShortensBothMintsOfAConnection(t *testing.T) {
	const chosen = 15 * time.Minute
	o := setupOAuth(t)
	setAccessTokenTTL(t, o, chosen)

	code := o.authorize(t, url.Values{"scope": {"read write offline_access"}})
	status, body := o.exchange(t, url.Values{"code": {code}})
	if status != http.StatusOK {
		t.Fatalf("code exchange → %d %v", status, body)
	}
	if got := accessTokenLifetime(t, body); !withinAMinuteOf(got, chosen) {
		t.Fatalf("the exchanged access token lives %s, want the chosen %s", got, chosen)
	}
	refresh, _ := body["refresh_token"].(string)
	if refresh == "" {
		t.Fatalf("offline_access exchange returned no refresh_token: %v", body)
	}

	status, renewed := o.renew(t, refresh, nil)
	if status != http.StatusOK {
		t.Fatalf("renewal → %d %v", status, renewed)
	}
	if got := accessTokenLifetime(t, renewed); !withinAMinuteOf(got, chosen) {
		t.Fatalf("the rotated access token lives %s, want the chosen %s: a rotation must not restore the default", got, chosen)
	}
}

// A change reaches the next token minted without a restart: a connection
// exchanged under the default renews under the new lifetime.
func TestAChangedAccessTokenTTLReachesTheNextRotation(t *testing.T) {
	o := setupOAuth(t)

	code := o.authorize(t, url.Values{"scope": {"read write offline_access"}})
	status, body := o.exchange(t, url.Values{"code": {code}})
	if status != http.StatusOK {
		t.Fatalf("code exchange → %d %v", status, body)
	}
	refresh, _ := body["refresh_token"].(string)
	if refresh == "" {
		t.Fatalf("offline_access exchange returned no refresh_token: %v", body)
	}

	const shortened = time.Hour
	setAccessTokenTTL(t, o, shortened)
	status, renewed := o.renew(t, refresh, nil)
	if status != http.StatusOK {
		t.Fatalf("renewal → %d %v", status, renewed)
	}
	if got := accessTokenLifetime(t, renewed); !withinAMinuteOf(got, shortened) {
		t.Fatalf("the rotation after the change lives %s, want the new %s", got, shortened)
	}
}

// An installation nobody tuned keeps minting the 30-day passport, the posture
// every connector had before this was a setting.
func TestAnUntouchedAccessTokenTTLKeepsThePassportDefault(t *testing.T) {
	o := setupOAuth(t)

	code := o.authorize(t, url.Values{"scope": {"read write offline_access"}})
	status, body := o.exchange(t, url.Values{"code": {code}})
	if status != http.StatusOK {
		t.Fatalf("code exchange → %d %v", status, body)
	}
	if got := accessTokenLifetime(t, body); got < 29*24*time.Hour {
		t.Fatalf("the exchanged access token lives %s, want the unchanged 30-day default", got)
	}
}

// A lifetime the passport mint would refuse is refused when it is SET, rather
// than on the first handshake of a connector nobody is watching.
func TestAnOutOfRangeAccessTokenTTLIsRefusedWhenSet(t *testing.T) {
	o := setupOAuth(t)
	for _, minutes := range []int{1, 91 * 24 * 60} {
		if status := o.Call(t, "PATCH", "/v1/installation/settings", integration.AnyMap{
			"oauth_access_token_ttl_minutes": minutes,
		}, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("a lifetime of %d minutes → %d, want 422", minutes, status)
		}
	}
}
