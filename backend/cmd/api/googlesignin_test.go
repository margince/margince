// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestGoogleSignInOptionsRefusesAMalformedPublicBaseURL(t *testing.T) {
	cfg := apiConfig{
		gmailClientID:     "cid",
		gmailClientSecret: "secret",
		connectorStateKey: "0123456789012345678901234567890123",
		publicBaseURL:     "https://admin:hunter2@crm.example.com",
	}
	if _, err := googleSignInOptions(cfg, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a refusal for a base URL carrying userinfo")
	}
}

func TestGoogleSignInOptionsAcceptsABareOrigin(t *testing.T) {
	cfg := apiConfig{
		gmailClientID:     "cid",
		gmailClientSecret: "secret",
		connectorStateKey: "0123456789012345678901234567890123",
		publicBaseURL:     "https://crm.example.com",
	}
	var stdout bytes.Buffer
	opts, err := googleSignInOptions(cfg, &stdout)
	if err != nil {
		t.Fatalf("googleSignInOptions: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("opts = %v, want exactly one Option", opts)
	}
	if !strings.Contains(stdout.String(), "google sign-in enabled") {
		t.Fatalf("boot log = %q, want the enabled line", stdout.String())
	}
}

// TestGoogleSignInOptionsSplitOriginUsesTheAPIHostForGoogleAndThePublicHostForTheLanding
// holds the same split gmail.go's callbackURL and connectors_outcome.go's
// landingURL already make: on a dev stack where the api and the SPA are
// different origins (--api-base-url set), the redirect_uri sent to Google
// must reach the api, while the human-facing landing after the round trip
// must reach the SPA — the two are NOT the same base.
func TestGoogleSignInOptionsSplitOriginUsesTheAPIHostForGoogleAndThePublicHostForTheLanding(t *testing.T) {
	cfg := apiConfig{
		gmailClientID:     "cid",
		gmailClientSecret: "secret",
		connectorStateKey: "0123456789012345678901234567890123",
		publicBaseURL:     "https://app.example.com",
		apiBaseURL:        "https://api.example.com",
	}
	opts, err := googleSignInOptions(cfg, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("googleSignInOptions: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("opts = %v, want exactly one Option", opts)
	}

	var s compose.Server
	opts[0](&s, nil)

	startReq := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/start", nil)
	startRec := httptest.NewRecorder()
	s.StartOidcSignIn(startRec, startReq, "google")
	loc, err := url.Parse(startRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if got := loc.Query().Get("redirect_uri"); got != "https://api.example.com/v1/auth/oidc/google/callback" {
		t.Fatalf("redirect_uri = %q, want the api host", got)
	}

	// The other half of the split: the callback's own landing must reach the
	// PUBLIC host, not the api host redirect_uri above used.
	callbackReq := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=x&state=y", nil)
	callbackRec := httptest.NewRecorder()
	s.OidcSignInCallback(callbackRec, callbackReq, "google", crmcontracts.OidcSignInCallbackParams{})
	failureLoc := callbackRec.Header().Get("Location")
	if !strings.HasPrefix(failureLoc, "https://app.example.com/") {
		t.Fatalf("failure redirect = %q, want it to land on the public host", failureLoc)
	}
}

func TestGoogleSignInOptionsSkipsValidationWhenUnconfigured(t *testing.T) {
	// No client id at all — the malformed base URL below must never be
	// inspected, matching gmailOptions' own posture of validating only what
	// it is actually about to wire.
	cfg := apiConfig{publicBaseURL: "https://admin:hunter2@crm.example.com"}
	if _, err := googleSignInOptions(cfg, &bytes.Buffer{}); err != nil {
		t.Fatalf("googleSignInOptions on an unconfigured deployment: %v", err)
	}
}
