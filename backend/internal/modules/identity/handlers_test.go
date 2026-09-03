// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

func TestGetAuthCapabilitiesReportsOIDCProviders(t *testing.T) {
	h := Handlers{}.WithOIDCProvidersEnabledFn(func(context.Context) ([]OIDCProviderConfig, error) {
		return []OIDCProviderConfig{{Key: "google", Label: "Continue with Google"}}, nil
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

// A policy read that fails must not leave a reader with no way in. The login
// screen renders from this response, so the degraded answer is the method every
// installation always has — password — and never a refusal. The ROUTES fail
// closed separately, so a short list here can admit nothing the policy refuses.
func TestGetAuthCapabilitiesReportsPasswordWhenTheProviderPolicyCannotBeRead(t *testing.T) {
	h := Handlers{}.WithOIDCProvidersEnabledFn(func(context.Context) ([]OIDCProviderConfig, error) {
		return nil, errors.New("the settings row is unreachable")
	})
	rec := httptest.NewRecorder()
	h.GetAuthCapabilities(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil))

	var body struct {
		Password      bool                          `json:"password"`
		OidcProviders []struct{ Key, Label string } `json:"oidc_providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.Password {
		t.Error("password sign-in was withheld because a provider policy read failed; it is the method that always remains")
	}
	if len(body.OidcProviders) != 0 {
		t.Errorf("oidc_providers = %v on a failed policy read, want none", body.OidcProviders)
	}
}

// The capabilities probe is anonymous and, once the provider policy is wired,
// reaches the database on every call. A flood must therefore cost the caller
// their buttons rather than costing the installation a pool connection each
// time — and password, the method that always remains, still has to render.
func TestGetAuthCapabilitiesStopsReadingTheProviderPolicyUnderAFlood(t *testing.T) {
	reads := 0
	h := NewHandlers(nil).WithOIDCProvidersEnabledFn(
		func(context.Context) ([]OIDCProviderConfig, error) {
			reads++
			return []OIDCProviderConfig{{Key: "google", Label: "Continue with Google"}}, nil
		},
	)

	var lastBody []byte
	for range 200 {
		rec := httptest.NewRecorder()
		h.GetAuthCapabilities(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("capabilities answered %d under load; the login screen must still render", rec.Code)
		}
		lastBody = rec.Body.Bytes()
	}
	if reads >= 200 {
		t.Errorf("the policy was read %d times in 200 anonymous requests; the budget bounds nothing", reads)
	}

	var body struct {
		Password      bool                          `json:"password"`
		OidcProviders []struct{ Key, Label string } `json:"oidc_providers"`
	}
	if err := json.Unmarshal(lastBody, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.Password {
		t.Error("password sign-in was withheld from a throttled caller; it is the method that always remains")
	}
}

// first_run is the anonymous probe's carry-through of the installation's own
// setup state (compose derives it from the same steps GetInstallationSetup
// reads); this module's job is only to put firstRunFn's answer on the wire.
func TestGetAuthCapabilitiesReportsFirstRunWhenSetupIsIncomplete(t *testing.T) {
	h := Handlers{}.WithFirstRunFn(func(context.Context) (bool, error) {
		return true, nil
	})
	rec := httptest.NewRecorder()
	h.GetAuthCapabilities(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil))

	var body struct {
		FirstRun bool `json:"first_run"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.FirstRun {
		t.Error("first_run = false for an installation whose setup is incomplete, want true")
	}
}

func TestGetAuthCapabilitiesReportsNoFirstRunWhenSetupIsComplete(t *testing.T) {
	h := Handlers{}.WithFirstRunFn(func(context.Context) (bool, error) {
		return false, nil
	})
	rec := httptest.NewRecorder()
	h.GetAuthCapabilities(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil))

	var body struct {
		FirstRun bool `json:"first_run"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.FirstRun {
		t.Error("first_run = true for an installation whose setup is already complete, want false")
	}
}

// A failed read must not turn an anonymous 200 into a 500, and must not
// mislabel a login screen as a welcome screen: the login UI still has to
// render, so the degraded answer is the one that offers ordinary sign-in.
func TestGetAuthCapabilitiesFirstRunDegradesToFalseWhenTheSignalCannotBeRead(t *testing.T) {
	h := Handlers{}.WithFirstRunFn(func(context.Context) (bool, error) {
		return false, errors.New("the installation workspace is unreachable")
	})
	rec := httptest.NewRecorder()
	h.GetAuthCapabilities(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("capabilities answered %d on a failed first-run read; the login screen must still render", rec.Code)
	}
	var body struct {
		FirstRun bool `json:"first_run"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.FirstRun {
		t.Error("first_run = true on a failed read; a reader who cannot be told must get the ordinary sign-in screen, not the welcome one")
	}
}

// The anonymous probe carries exactly the fields AuthCapabilities declares —
// never a step name, a configured value, or anything about an account. This
// pins the RESPONSE SHAPE itself, so a future field added anywhere in this
// handler cannot widen what an anonymous caller learns without failing here.
func TestGetAuthCapabilitiesDisclosesNothingBeyondItsFixedFields(t *testing.T) {
	h := Handlers{}.WithFirstRunFn(func(context.Context) (bool, error) {
		return true, nil
	})
	rec := httptest.NewRecorder()
	h.GetAuthCapabilities(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil))

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	allowed := map[string]bool{
		"password": true, "password_reset": true, "oidc_providers": true,
		"first_run": true, "release_version": true,
	}
	for key := range body {
		if !allowed[key] {
			t.Errorf("capabilities response carries unexpected field %q; an anonymous caller must learn nothing beyond the login screen's fixed vocabulary", key)
		}
	}
}
