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
