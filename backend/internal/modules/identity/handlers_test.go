// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

func TestGetAuthCapabilitiesReportsOIDCProviders(t *testing.T) {
	h := Handlers{}.WithOIDCCapabilitiesFn(func() []OIDCProviderConfig {
		return []OIDCProviderConfig{{Key: "google", Label: "Continue with Google"}}
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil)
	rec := httptest.NewRecorder()

	h.GetAuthCapabilities(rec, req)

	var body struct {
		OidcProviders []struct{ Key, Label string } `json:"oidc_providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.OidcProviders) != 1 || body.OidcProviders[0].Key != "google" {
		t.Fatalf("oidc_providers = %+v", body.OidcProviders)
	}
}

func TestGetAuthCapabilitiesEmptyWhenUnconfigured(t *testing.T) {
	h := Handlers{}
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil)
	rec := httptest.NewRecorder()

	h.GetAuthCapabilities(rec, req)

	var body struct {
		OidcProviders []struct{ Key, Label string } `json:"oidc_providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.OidcProviders) != 0 {
		t.Fatalf("oidc_providers = %+v, want empty", body.OidcProviders)
	}
}

func TestResetRateLimitsReopensASpentBucket(t *testing.T) {
	h := NewHandlers(&Service{})
	for range 3 {
		h.resetPerEmail.Allow("a@b.test|127.0.0.1")
	}
	if h.resetPerEmail.Allow("a@b.test|127.0.0.1") {
		t.Fatal("the 4th attempt within the 3/hour ceiling must be refused; the bucket is spent")
	}

	h.ResetRateLimits()

	if !h.resetPerEmail.Allow("a@b.test|127.0.0.1") {
		t.Error("resetPerEmail still refuses after ResetRateLimits; the bucket was not cleared")
	}
}

// TestResetRateLimitsOnAHandlerSetWithoutBucketsIsANoOp: the caller is the
// non-production data reset, and a nil-limiter panic there would surface to the
// operator as an opaque 500 on a wipe that had already finished. A handler set
// with no buckets has nothing to clear and must say so by returning quietly.
func TestResetRateLimitsOnAHandlerSetWithoutBucketsIsANoOp(t *testing.T) {
	var h Handlers
	h.ResetRateLimits()
	// All four, not a sample: ResetRateLimits iterates the whole set, so an edit
	// that allocated only an unchecked bucket would slip past a partial check.
	for name, bucket := range map[string]*ratelimit.Limiter{
		"loginFailures": h.loginFailures,
		"loginPerIP":    h.loginPerIP,
		"resetPerEmail": h.resetPerEmail,
		"resetPerIP":    h.resetPerIP,
	} {
		if bucket != nil {
			t.Errorf("a zero-value handler set grew a %s limiter; this case exists precisely because it has none", name)
		}
	}
}
