// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// The Google app over HTTP, and the setup report that reads it.
//
// The store's own suite (googleapp_integration_test.go) proves the sealing and
// the retirement. This one proves the TRANSPORT: the status codes, that the
// secret never travels back, that an unentitled seat is refused, and that a
// role with nowhere to seal answers 503 rather than pretending the surface does
// not exist. Those are the lines a store test cannot reach, and the surface
// shipped once already with a defect no store test could have seen.

import (
	"crypto/rand"
	"net/http"
	"net/url"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose"
	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/keyvault"
)

// setupGoogleAppHTTP boots the api composition WITH a vault, which is what puts
// the Google-app surface on the server at all.
func setupGoogleAppHTTP(t *testing.T) *apptest.AppEnv {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating a test root key: %v", err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: key, Pool: apptest.EarlyPool(t)})
	if err != nil {
		t.Fatalf("building the local vault: %v", err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithKeyvault(vault))
	e.BootstrapWorkspace(t)
	return e
}

const (
	httpClientID = "111-abc.apps.googleusercontent.com"
	httpSecret   = "GOCSPX-over-the-wire"
)

func TestGoogleAppOverHTTP(t *testing.T) {
	e := setupGoogleAppHTTP(t)

	// An installation that has not set one up has not failed at anything.
	var app crmcontracts.GoogleApp
	if status := e.Call(t, "GET", "/v1/installation/google-app", nil, nil, &app); status != http.StatusOK {
		t.Fatalf("GET google-app on a fresh installation → %d, want 200", status)
	}
	if app.Configured {
		t.Fatal("a fresh installation reports a configured Google app")
	}

	// Storing answers 204 with no body: the only thing left to echo is the
	// secret, and a response body is what proxies log and browsers cache.
	secret := httpSecret
	body := crmcontracts.GoogleAppInput{ClientId: httpClientID, ClientSecret: &secret}
	if status := e.Call(t, "PUT", "/v1/installation/google-app", body, nil, nil); status != http.StatusNoContent {
		t.Fatalf("PUT google-app → %d, want 204", status)
	}

	// The read carries the client id — an operator has to see WHICH app the
	// installation uses to check it against the Google console — and the
	// generated response type has nowhere to put a secret, which is the
	// contract's writeOnly doing its job.
	if status := e.Call(t, "GET", "/v1/installation/google-app", nil, nil, &app); status != http.StatusOK {
		t.Fatalf("GET google-app after storing → %d, want 200", status)
	}
	if !app.Configured || app.ClientId != httpClientID {
		t.Fatalf("after storing, the app reads %+v, want configured with the client id", app)
	}

	// Removing clears it, and removing an absent one still succeeds: the caller
	// asked for a state and that state already holds.
	if status := e.Call(t, "DELETE", "/v1/installation/google-app", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("DELETE google-app → %d, want 204", status)
	}
	if status := e.Call(t, "GET", "/v1/installation/google-app", nil, nil, &app); status != http.StatusOK || app.Configured {
		t.Fatalf("after removal the app reads %+v (status %d), want empty", app, status)
	}
	if status := e.Call(t, "DELETE", "/v1/installation/google-app", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("DELETE of an absent app → %d, want 204", status)
	}
}

// The refusals, each for a value that would otherwise read as configured while
// authenticating nothing.
func TestGoogleAppOverHTTPRefusesAnUnusableApp(t *testing.T) {
	e := setupGoogleAppHTTP(t)
	secret := httpSecret
	blank := ""
	for _, tc := range []struct {
		name string
		body crmcontracts.GoogleAppInput
	}{
		{"a client id Google would never issue", crmcontracts.GoogleAppInput{ClientId: "not-a-google-client", ClientSecret: &secret}},
		{"an empty secret", crmcontracts.GoogleAppInput{ClientId: httpClientID, ClientSecret: &blank}},
		{"no secret at all", crmcontracts.GoogleAppInput{ClientId: httpClientID}},
		{"an empty client id", crmcontracts.GoogleAppInput{ClientId: "", ClientSecret: &secret}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if status := e.Call(t, "PUT", "/v1/installation/google-app", tc.body, nil, nil); status != http.StatusUnprocessableEntity {
				t.Fatalf("PUT %s → %d, want 422", tc.name, status)
			}
		})
	}
	// None of them left anything behind.
	var app crmcontracts.GoogleApp
	if status := e.Call(t, "GET", "/v1/installation/google-app", nil, nil, &app); status != http.StatusOK || app.Configured {
		t.Fatalf("a refused write left the app reading %+v (status %d), want empty", app, status)
	}
}

// A read seat may LOOK and may not touch.
//
// Both halves are the declared contract rather than an accident: the RBAC
// baseline grants `capture_settings` read to every role, read_only included —
// the client id is not a secret and an operator has to be able to see which app
// the installation uses — while the seat TIER is what refuses the writes. So
// this asserts the read succeeds as well as that the writes are refused; a test
// that only checked the 403s would pass just as happily if the read had been
// locked down by mistake, and would call that a success.
func TestGoogleAppOverHTTPLetsAReadSeatLookAndNotTouch(t *testing.T) {
	e := setupGoogleAppHTTP(t)
	secret := httpSecret
	// Stored while the seat can still write, so the read below has something to
	// report and cannot pass by finding nothing.
	if status := e.Call(t, "PUT", "/v1/installation/google-app",
		crmcontracts.GoogleAppInput{ClientId: httpClientID, ClientSecret: &secret}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("PUT google-app as admin → %d, want 204", status)
	}
	e.SetWorkspaceSeat(t, "read")

	var app crmcontracts.GoogleApp
	if status := e.Call(t, "GET", "/v1/installation/google-app", nil, nil, &app); status != http.StatusOK {
		t.Fatalf("GET google-app on a read seat → %d, want 200: the baseline grants capture_settings read to every role", status)
	}
	if !app.Configured || app.ClientId != httpClientID {
		t.Fatalf("a read seat reads %+v, want the stored app", app)
	}
	for _, tc := range []struct {
		method string
		body   any
	}{
		{"PUT", crmcontracts.GoogleAppInput{ClientId: httpClientID, ClientSecret: &secret}},
		{"DELETE", nil},
	} {
		t.Run(tc.method, func(t *testing.T) {
			if status := e.Call(t, tc.method, "/v1/installation/google-app", tc.body, nil, nil); status != http.StatusForbidden {
				t.Fatalf("%s google-app on a read seat → %d, want 403", tc.method, status)
			}
		})
	}
}

// The setup report is what onboarding gates on, so it has to move when the
// installation does rather than reporting a constant.
func TestInstallationSetupOverHTTPTracksTheGoogleApp(t *testing.T) {
	e := setupGoogleAppHTTP(t)

	googleStep := func(t *testing.T) crmcontracts.InstallationSetupStep {
		t.Helper()
		var setup crmcontracts.InstallationSetup
		if status := e.Call(t, "GET", "/v1/installation/setup", nil, nil, &setup); status != http.StatusOK {
			t.Fatalf("GET installation/setup → %d, want 200", status)
		}
		for _, s := range setup.Steps {
			if s.Step == crmcontracts.InstallationSetupStepStepGoogleApp {
				return s
			}
		}
		t.Fatal("the setup report names no google_app step, so onboarding has nothing to gate on")
		return crmcontracts.InstallationSetupStep{}
	}

	if step := googleStep(t); step.Configured {
		t.Fatal("a fresh installation reports the Google app as configured")
	}
	secret := httpSecret
	if status := e.Call(t, "PUT", "/v1/installation/google-app",
		crmcontracts.GoogleAppInput{ClientId: httpClientID, ClientSecret: &secret}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("PUT google-app → %d, want 204", status)
	}
	if step := googleStep(t); !step.Configured {
		t.Fatal("the setup report still calls the Google app unconfigured after it was stored")
	}
	if status := e.Call(t, "DELETE", "/v1/installation/google-app", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("DELETE google-app → %d, want 204", status)
	}
	if step := googleStep(t); step.Configured {
		t.Fatal("the setup report still calls the Google app configured after it was removed")
	}
}

// A role that composed no vault has nowhere to seal a secret. That is the
// operator's to fix, so the surface says 503 — not 501, which would claim this
// BUILD does not implement the operation and send an integrator looking for a
// newer version that cannot help.
func TestGoogleAppOverHTTPWithoutAVaultIsUnavailableNotUnimplemented(t *testing.T) {
	e := apptest.SetupApp(t) // no compose.WithKeyvault
	e.BootstrapWorkspace(t)
	secret := httpSecret
	for _, tc := range []struct {
		method string
		body   any
	}{
		{"GET", nil},
		{"PUT", crmcontracts.GoogleAppInput{ClientId: httpClientID, ClientSecret: &secret}},
		{"DELETE", nil},
	} {
		t.Run(tc.method, func(t *testing.T) {
			if status := e.Call(t, tc.method, "/v1/installation/google-app", tc.body, nil, nil); status != http.StatusServiceUnavailable {
				t.Fatalf("%s google-app with no vault → %d, want 503", tc.method, status)
			}
		})
	}
}

// setupGoogleAppWithEnvApp composes a vault AND an environment-supplied Google
// app, which is what registers the Google connectors — the shape in which a
// STORED app can actually be used, and so the only one where its precedence
// over the environment is observable.
func setupGoogleAppWithEnvApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating a test root key: %v", err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: key, Pool: apptest.EarlyPool(t)})
	if err != nil {
		t.Fatalf("building the local vault: %v", err)
	}
	e := apptest.SetupAppWithOptions(t,
		compose.WithKeyvault(vault),
		compose.WithGmailCapture(compose.GmailConfig{
			ClientID:      "env-client.apps.googleusercontent.com",
			ClientSecret:  "GOCSPX-from-the-environment",
			StateKey:      "0123456789abcdef0123456789abcdef",
			PublicBaseURL: "https://app.example",
		}, compose.CaptureConfig{}),
	)
	e.BootstrapWorkspace(t)
	return e
}

// The whole point of storing an app: once one is set in Settings, it is the app
// the consent flow uses — the environment's is no longer what a person is sent
// to Google with. Asserted on the authorize URL, because the client id in it is
// the only place the choice becomes visible from outside.
//
// Both providers. Gmail and Calendar authorize SEPARATELY through one app, and
// they build their consent pair in two different functions — so a stored app
// reaching one and not the other is a live failure mode, not a hypothetical.
func TestAStoredGoogleAppOutranksTheEnvironmentForBothProviders(t *testing.T) {
	e := setupGoogleAppWithEnvApp(t)

	authorizeClientID := func(t *testing.T, provider string) string {
		t.Helper()
		var resp crmcontracts.ConnectConnectorResponse
		if status := e.Call(t, "POST", "/v1/connectors/"+provider+"/connect", nil, nil, &resp); status != http.StatusOK {
			t.Fatalf("POST connect %s → %d, want 200", provider, status)
		}
		if resp.AuthorizeUrl == nil {
			t.Fatalf("connect %s returned no authorize url", provider)
		}
		u, err := url.Parse(*resp.AuthorizeUrl)
		if err != nil {
			t.Fatalf("connect %s returned an unparseable authorize url %q: %v", provider, *resp.AuthorizeUrl, err)
		}
		return u.Query().Get("client_id")
	}

	// Before: the environment's app, which is what makes the assertion after
	// storing mean something rather than passing on a constant.
	for _, provider := range []string{"gmail", "gcal"} {
		if got := authorizeClientID(t, provider); got != "env-client.apps.googleusercontent.com" {
			t.Fatalf("%s consent uses client_id %q before an app is stored, want the environment's", provider, got)
		}
	}

	secret := httpSecret
	if status := e.Call(t, "PUT", "/v1/installation/google-app",
		crmcontracts.GoogleAppInput{ClientId: httpClientID, ClientSecret: &secret}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("PUT google-app → %d, want 204", status)
	}

	// After: the stored one, with no restart — the resolution is per request.
	for _, provider := range []string{"gmail", "gcal"} {
		if got := authorizeClientID(t, provider); got != httpClientID {
			t.Errorf("%s consent uses client_id %q after an app was stored, want the stored one — an app set in Settings that never reaches Google is the whole feature not working", provider, got)
		}
	}

	// And removing it falls back rather than breaking: the environment still has
	// an app, and an installation that clears its own must not lose the flow.
	if status := e.Call(t, "DELETE", "/v1/installation/google-app", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("DELETE google-app → %d, want 204", status)
	}
	for _, provider := range []string{"gmail", "gcal"} {
		if got := authorizeClientID(t, provider); got != "env-client.apps.googleusercontent.com" {
			t.Errorf("%s consent uses client_id %q after the stored app was removed, want the environment's back", provider, got)
		}
	}
}

// The AI step of the setup report, which is the other half of what onboarding
// gates on — and the half with the rule worth pinning: a BINDING alone does not
// make the step configured. Every cloud vendor the binding names needs a
// credential too, because a bound installation with no key fails on its first
// real call, and reporting it ready would send an admin through onboarding into
// a cold start that cannot run.
func TestInstallationSetupNeedsBothABindingAndItsKey(t *testing.T) {
	e := setupGoogleAppHTTP(t)

	aiStep := func(t *testing.T) crmcontracts.InstallationSetupStep {
		t.Helper()
		var setup crmcontracts.InstallationSetup
		if status := e.Call(t, "GET", "/v1/installation/setup", nil, nil, &setup); status != http.StatusOK {
			t.Fatalf("GET installation/setup → %d, want 200", status)
		}
		for _, s := range setup.Steps {
			if s.Step == crmcontracts.InstallationSetupStepStepAiModels {
				return s
			}
		}
		t.Fatal("the setup report names no ai_models step, so onboarding has nothing to gate on")
		return crmcontracts.InstallationSetupStep{}
	}

	if step := aiStep(t); step.Configured {
		t.Fatal("a fresh installation reports the AI step as configured with nothing bound")
	}

	// Bind a CLOUD vendor on every tier. Cloud is the point: it is the case that
	// needs a credential, and a local-only binding would pass this step with no
	// key at all — which is correct, and would prove nothing here.
	binding := crmcontracts.AiTierBinding{Provider: "gemini", Model: "gemini-3.1-flash-lite"}
	routing := crmcontracts.AiRouting{
		Profile: "eu_hosted",
		Tiers: map[string]crmcontracts.AiTierBinding{
			"local_small": binding, "cheap_cloud": binding, "premium": binding,
		},
		Embeddings: crmcontracts.AiEmbeddingsBinding{Provider: "gemini", Model: "gemini-embed-1"},
	}
	if status := e.Call(t, "PUT", "/v1/ai/routing", routing, nil, nil); status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("PUT ai/routing → %d, want 200 or 204", status)
	}

	// Bound, and still NOT configured: the vendor has no key.
	if step := aiStep(t); step.Configured {
		t.Error("the AI step reads configured with a cloud vendor bound and no key for it — onboarding would wave the admin through into a cold start that cannot make a call")
	}

	key := "AIza-test-key"
	if status := e.Call(t, "PUT", "/v1/ai/provider-keys/gemini",
		crmcontracts.AiProviderKeyInput{ApiKey: &key}, nil, nil); status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("PUT provider key → %d, want 200 or 204", status)
	}

	// Both halves present: now it is configured.
	if step := aiStep(t); !step.Configured {
		t.Error("the AI step still reads unconfigured with a binding AND its key stored")
	}
}
