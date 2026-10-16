// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// The connector OAuth apps over HTTP, and the setup report that reads them.
//
// The store's own suite (connectorapp_integration_test.go) proves the sealing and
// the retirement. This one proves the TRANSPORT: the status codes, that the
// secret never travels back, that an unentitled seat is refused, and that a
// role with nowhere to seal answers 503 rather than pretending the surface does
// not exist. Those are the lines a store test cannot reach, and the surface
// shipped once already with a defect no store test could have seen.

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// setupConnectorAppHTTP boots the api composition WITH a vault, which is what
// puts the OAuth-app surface on the server at all.
func setupConnectorAppHTTP(t *testing.T) *apptest.AppEnv {
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
	e := setupConnectorAppHTTP(t)

	// An installation that has not set one up has not failed at anything.
	var app crmcontracts.ConnectorApp
	if status := e.Call(t, "GET", "/v1/installation/oauth-apps/google", nil, nil, &app); status != http.StatusOK {
		t.Fatalf("GET the app on a fresh installation → %d, want 200", status)
	}
	if app.Configured {
		t.Fatal("a fresh installation reports a configured Google app")
	}

	// Storing answers 204 with no body: the only thing left to echo is the
	// secret, and a response body is what proxies log and browsers cache.
	secret := httpSecret
	body := crmcontracts.ConnectorAppInput{ClientId: httpClientID, ClientSecret: &secret}
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/google", body, nil, nil); status != http.StatusNoContent {
		t.Fatalf("PUT the app → %d, want 204", status)
	}

	// The read carries the client id — an operator has to see WHICH app the
	// installation uses to check it against the Google console — and the
	// generated response type has nowhere to put a secret, which is the
	// contract's writeOnly doing its job.
	if status := e.Call(t, "GET", "/v1/installation/oauth-apps/google", nil, nil, &app); status != http.StatusOK {
		t.Fatalf("GET the app after storing → %d, want 200", status)
	}
	if !app.Configured || app.ClientId != httpClientID {
		t.Fatalf("after storing, the app reads %+v, want configured with the client id", app)
	}

	// Removing clears it, and removing an absent one still succeeds: the caller
	// asked for a state and that state already holds.
	if status := e.Call(t, "DELETE", "/v1/installation/oauth-apps/google", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("DELETE the app → %d, want 204", status)
	}
	if status := e.Call(t, "GET", "/v1/installation/oauth-apps/google", nil, nil, &app); status != http.StatusOK || app.Configured {
		t.Fatalf("after removal the app reads %+v (status %d), want empty", app, status)
	}
	if status := e.Call(t, "DELETE", "/v1/installation/oauth-apps/google", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("DELETE of an absent app → %d, want 204", status)
	}
}

// The refusals, each for a value that would otherwise read as configured while
// authenticating nothing.
func TestGoogleAppOverHTTPRefusesAnUnusableApp(t *testing.T) {
	e := setupConnectorAppHTTP(t)
	secret := httpSecret
	blank := ""
	for _, tc := range []struct {
		name string
		body crmcontracts.ConnectorAppInput
	}{
		{"a client id Google would never issue", crmcontracts.ConnectorAppInput{ClientId: "not-a-google-client", ClientSecret: &secret}},
		{"an empty secret", crmcontracts.ConnectorAppInput{ClientId: httpClientID, ClientSecret: &blank}},
		{"no secret at all", crmcontracts.ConnectorAppInput{ClientId: httpClientID}},
		{"an empty client id", crmcontracts.ConnectorAppInput{ClientId: "", ClientSecret: &secret}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/google", tc.body, nil, nil); status != http.StatusUnprocessableEntity {
				t.Fatalf("PUT %s → %d, want 422", tc.name, status)
			}
		})
	}
	// None of them left anything behind.
	var app crmcontracts.ConnectorApp
	if status := e.Call(t, "GET", "/v1/installation/oauth-apps/google", nil, nil, &app); status != http.StatusOK || app.Configured {
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
	e := setupConnectorAppHTTP(t)
	secret := httpSecret
	// Stored while the seat can still write, so the read below has something to
	// report and cannot pass by finding nothing.
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/google",
		crmcontracts.ConnectorAppInput{ClientId: httpClientID, ClientSecret: &secret}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("PUT the app as admin → %d, want 204", status)
	}
	e.SetWorkspaceSeat(t, "read")

	var app crmcontracts.ConnectorApp
	if status := e.Call(t, "GET", "/v1/installation/oauth-apps/google", nil, nil, &app); status != http.StatusOK {
		t.Fatalf("GET the app on a read seat → %d, want 200: the baseline grants capture_settings read to every role", status)
	}
	if !app.Configured || app.ClientId != httpClientID {
		t.Fatalf("a read seat reads %+v, want the stored app", app)
	}
	for _, tc := range []struct {
		method string
		body   any
	}{
		{"PUT", crmcontracts.ConnectorAppInput{ClientId: httpClientID, ClientSecret: &secret}},
		{"DELETE", nil},
	} {
		t.Run(tc.method, func(t *testing.T) {
			if status := e.Call(t, tc.method, "/v1/installation/oauth-apps/google", tc.body, nil, nil); status != http.StatusForbidden {
				t.Fatalf("%s google-app on a read seat → %d, want 403", tc.method, status)
			}
		})
	}
}

// readSetup is the whole report, which is what `complete` has to be read from:
// the field is the server's own verdict and a test recomputing it from the step
// list would be asserting its own arithmetic rather than the installation's
// policy.
func readSetup(t *testing.T, e *apptest.AppEnv) crmcontracts.InstallationSetup {
	t.Helper()
	var setup crmcontracts.InstallationSetup
	if status := e.Call(t, "GET", "/v1/installation/setup", nil, nil, &setup); status != http.StatusOK {
		t.Fatalf("GET installation/setup → %d, want 200", status)
	}
	return setup
}

// setupStep is one named step out of the report. A step the report never names
// is fatal rather than a zero value: every assertion below is about what the
// step SAYS, and a silently absent one would read as "not configured, not
// blocking" and pass.
func setupStep(t *testing.T, e *apptest.AppEnv, which crmcontracts.InstallationSetupStepStep) crmcontracts.InstallationSetupStep {
	t.Helper()
	for _, s := range readSetup(t, e).Steps {
		if s.Step == which {
			return s
		}
	}
	t.Fatalf("the setup report names no %q step, so no surface can say whether it is outstanding", which)
	return crmcontracts.InstallationSetupStep{}
}

// The setup report is what onboarding reads, so it has to move when the
// installation does rather than reporting a constant.
func TestInstallationSetupOverHTTPTracksTheGoogleApp(t *testing.T) {
	e := setupConnectorAppHTTP(t)

	if step := setupStep(t, e, crmcontracts.InstallationSetupStepStepOauthApp); step.Configured {
		t.Fatal("a fresh installation reports the Google app as configured")
	}
	secret := httpSecret
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/google",
		crmcontracts.ConnectorAppInput{ClientId: httpClientID, ClientSecret: &secret}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("PUT the app → %d, want 204", status)
	}
	if step := setupStep(t, e, crmcontracts.InstallationSetupStepStepOauthApp); !step.Configured {
		t.Fatal("the setup report still calls the Google app unconfigured after it was stored")
	}
	if status := e.Call(t, "DELETE", "/v1/installation/oauth-apps/google", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("DELETE the app → %d, want 204", status)
	}
	if step := setupStep(t, e, crmcontracts.InstallationSetupStepStepOauthApp); step.Configured {
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
		{"PUT", crmcontracts.ConnectorAppInput{ClientId: httpClientID, ClientSecret: &secret}},
		{"DELETE", nil},
	} {
		t.Run(tc.method, func(t *testing.T) {
			if status := e.Call(t, tc.method, "/v1/installation/oauth-apps/google", tc.body, nil, nil); status != http.StatusServiceUnavailable {
				t.Fatalf("%s the app with no vault → %d, want 503", tc.method, status)
			}
		})
	}

	// And the 503 must never arrive BEFORE the grant is judged, or the status
	// code becomes an oracle: a seat with no capture grant would get 503 where
	// no vault exists and 403 where one does, so anybody with a session could
	// read off whether a vault root key is configured.
	//
	// This is the composition where that is observable — the same request, the
	// same missing vault, a seat that may not write.
	t.Run("a seat that may not write is refused before the vault is considered", func(t *testing.T) {
		e.SetWorkspaceSeat(t, "read")
		if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/google",
			crmcontracts.ConnectorAppInput{ClientId: httpClientID, ClientSecret: &secret}, nil, nil); status != http.StatusForbidden {
			t.Fatalf("PUT as a read seat with no vault → %d, want 403 — the wiring answered before the grant did", status)
		}
	})
}

// setupGoogleAppWithEnvApp composes a vault and an environment-supplied Google
// app, which registers the Google connectors. In that shape the environment's
// precedence over a stored app is observable. It returns the vault, so a test
// can record a stored app the way the store does.
func setupGoogleAppWithEnvApp(t *testing.T, opts ...compose.Option) (*apptest.AppEnv, keyvault.Vault) {
	t.Helper()
	return apptest.SetupGoogleConnect(t, "env-client.apps.googleusercontent.com", "GOCSPX-from-the-environment", opts...)
}

// storedAppCtx records and reads a stored app as the system, on the
// installation's workspace. The app is stored before the environment's
// arrives, which no request of this composition can do.
func storedAppCtx(t *testing.T, e *apptest.AppEnv) context.Context {
	t.Helper()
	ws := apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool)
	return principal.SystemActing(principal.WithWorkspaceID(context.Background(), ws), "test-stored-app")
}

// An app the environment supplies is the one the consent flow uses, over an app
// stored before. It locks the stored one: a write or a removal is refused, and
// the stored app is left as it was.
//
// Both providers. Gmail and Calendar authorize separately through one app, and
// build their consent pair in two functions. So the rule could reach one and
// not the other.
func TestAConnectorAppFromEnvBeatsTheStoredOne(t *testing.T) {
	e, vault := setupGoogleAppWithEnvApp(t)
	stored := capture.NewConnectorAppStore(compose.NewSettingsStore(e.Pool), vault, nil)
	if err := stored.Set(storedAppCtx(t, e), capture.AppProviderGoogle, httpClientID, httpSecret, ""); err != nil {
		t.Fatalf("recording a stored app: %v", err)
	}

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
	for _, provider := range []string{"gmail", "gcal"} {
		if got := authorizeClientID(t, provider); got != "env-client.apps.googleusercontent.com" {
			t.Errorf("%s consent uses client_id %q, want the environment's over the stored one", provider, got)
		}
	}

	var app crmcontracts.ConnectorApp
	if status := e.Call(t, "GET", "/v1/installation/oauth-apps/google", nil, nil, &app); status != http.StatusOK ||
		app.ClientId != "env-client.apps.googleusercontent.com" || app.Source != crmcontracts.ConnectorAppSourceEnvironment {
		t.Fatalf("GET the app → %d %+v, want the environment's app in use", status, app)
	}
	if lock, ok := appLock(app, capture.GoogleAppKey); !ok || lock.Origin.Source != crmcontracts.SettingOriginSourceEnv {
		t.Errorf("locks = %+v, want %s locked by the environment", app.Locks, capture.GoogleAppKey)
	}

	secret := httpSecret
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/google",
		crmcontracts.ConnectorAppInput{ClientId: "another.apps.googleusercontent.com", ClientSecret: &secret}, nil, nil); status != http.StatusConflict {
		t.Errorf("PUT over the environment's app → %d, want 409", status)
	}
	// Locked before the value is judged: an id Google never issues is still a 409.
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/google",
		crmcontracts.ConnectorAppInput{ClientId: "not-a-client-id", ClientSecret: &secret}, nil, nil); status != http.StatusConflict {
		t.Errorf("an invalid PUT over the environment's app → %d, want 409", status)
	}
	if status := e.Call(t, "DELETE", "/v1/installation/oauth-apps/google", nil, nil, nil); status != http.StatusConflict {
		t.Errorf("DELETE under the environment's app → %d, want 409", status)
	}
	kept, err := stored.Read(storedAppCtx(t, e), capture.AppProviderGoogle)
	if err != nil || !kept.Configured || kept.ClientID != httpClientID {
		t.Errorf("the stored app reads %+v %v, want it left as it was", kept, err)
	}
}

// The Microsoft app takes the same lock from its own environment pair.
func TestAMicrosoftAppFromEnvLocksTheStoredOne(t *testing.T) {
	e, _ := setupGoogleAppWithEnvApp(t, compose.WithGraphCapture(compose.GraphConfig{
		ClientID: "00000000-0000-0000-0000-00000000e0e0", ClientSecret: "from-the-environment",
	}))
	secret := httpSecret
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/microsoft",
		crmcontracts.ConnectorAppInput{ClientId: "00000000-0000-0000-0000-000000000001", ClientSecret: &secret}, nil, nil); status != http.StatusConflict {
		t.Errorf("PUT over the environment's Microsoft app → %d, want 409", status)
	}
}

// bindCloudRouting binds a cloud vendor on every tier this rule needs — one
// bound cloud tier is enough for CloudProvidersBound to name it, and the
// routing surface carries more than these three.
//
// Cloud is the point: it is the case that needs a credential, and a local-only
// binding would report the AI step configured with no key at all — which is
// correct, and would prove nothing about the rule the callers are here for.
func bindCloudRouting(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
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
}

// storeCloudProviderKey gives the vendor bindCloudRouting bound its credential —
// the second half of what makes the AI step configured.
func storeCloudProviderKey(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	key := "AIza-test-key"
	if status := e.Call(t, "PUT", "/v1/ai/provider-keys/gemini",
		crmcontracts.AiProviderKeyInput{ApiKey: &key}, nil, nil); status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("PUT provider key → %d, want 200 or 204", status)
	}
}

// ONLY the model binding blocks first run.
//
// An installation deployed without a Google app is fully usable — password
// sign-in, no external provider — so a Google app has no place gating the
// company form onboarding writes from.
//
// The step is still REPORTED, so a surface can say it is outstanding, and it
// is still second, so nothing here weakens the order
// TestTheSetupReportListsTheStepsInTheOrderOnboardingWalksThem pins. What this
// pins is the policy: unconfigured and non-blocking, and an installation that
// has bound a model is complete without ever touching Google.
//
// It reads every step rather than only the Google one on purpose: a test naming
// just `google_app` would not fail the day a third step arrived blocking.
//
// Both halves are asserted. A loop that only rejects blockers OTHER than
// ai_models proves nothing about ai_models itself, and reading that back out of
// `complete` would be reasoning from the server's own arithmetic — which is
// exactly what readSetup exists to avoid.
func TestOnlyTheModelBindingBlocksFirstRun(t *testing.T) {
	e := setupConnectorAppHTTP(t)

	fresh := readSetup(t, e)
	for _, step := range fresh.Steps {
		if step.Blocking && step.Step != crmcontracts.InstallationSetupStepStepAiModels {
			t.Errorf("step %q blocks first run; only ai_models may, because it is the only one onboarding can ask for", step.Step)
		}
	}
	if step := setupStep(t, e, crmcontracts.InstallationSetupStepStepAiModels); !step.Blocking {
		t.Error("ai_models does not block first run, so nothing gates the cold-start read the product cannot run without")
	}
	if fresh.Complete {
		t.Fatal("a fresh installation reports setup complete with no model bound")
	}
	bindCloudRouting(t, e)
	storeCloudProviderKey(t, e)

	// ONE report answers both halves. Completeness and the Google step read from
	// two separate GETs would let the sentence below describe a pairing neither
	// response ever carried.
	setup := readSetup(t, e)
	if !setup.Complete {
		t.Errorf("setup reads incomplete with a model bound and keyed: %+v", setup.Steps)
	}
	for _, step := range setup.Steps {
		if step.Step == crmcontracts.InstallationSetupStepStepOauthApp && step.Configured {
			t.Fatal("the Google app reads configured though none was ever stored, so completeness in this same report proves nothing about skipping it")
		}
	}
}

// The AI step of the setup report, which is the other half of what onboarding
// reads — and the half with the rule worth pinning: a BINDING alone does not
// make the step configured. Every cloud vendor the binding names needs a
// credential too, because a bound installation with no key fails on its first
// real call, and reporting it ready would send an admin through onboarding into
// a cold start that cannot run.
func TestInstallationSetupNeedsBothABindingAndItsKey(t *testing.T) {
	e := setupConnectorAppHTTP(t)

	if step := setupStep(t, e, crmcontracts.InstallationSetupStepStepAiModels); step.Configured {
		t.Fatal("a fresh installation reports the AI step as configured with nothing bound")
	}

	bindCloudRouting(t, e)

	// Bound, and still NOT configured: the vendor has no key.
	if step := setupStep(t, e, crmcontracts.InstallationSetupStepStepAiModels); step.Configured {
		t.Error("the AI step reads configured with a cloud vendor bound and no key for it — onboarding would wave the admin through into a cold start that cannot make a call")
	}

	storeCloudProviderKey(t, e)

	// Both halves present: now it is configured.
	if step := setupStep(t, e, crmcontracts.InstallationSetupStepStepAiModels); !step.Configured {
		t.Error("the AI step still reads unconfigured with a binding AND its key stored")
	}
}

// A stored app whose secret will not open is a DIFFERENT answer from one that
// was never configured, and the connect surface has to say so.
//
// 501 would tell an operator this deployment does not serve Gmail at all, and
// send them to configure a second app while the one they already stored sits
// there unopenable — the actual repair is the vault root key, or re-entering the
// secret. So the branch exists to avoid exactly that, and this holds it.
//
// The fault is injected at the vault rather than mocked: dropping the sealed
// bytes is what a rotated root key or a restored-without-the-vault database
// looks like from here, and it exercises the real unseal failing for the real
// reason.
func TestAStoredAppWhoseSecretWillNotOpenIsNotReportedAsUnconfigured(t *testing.T) {
	e, _ := apptest.SetupGoogleConnect(t, "", "")
	secret := httpSecret
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/google",
		crmcontracts.ConnectorAppInput{ClientId: httpClientID, ClientSecret: &secret}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("PUT the app → %d, want 204", status)
	}
	// It works before the fault, or what follows proves nothing.
	if status := e.Call(t, "POST", "/v1/connectors/gmail/connect", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("connect with a readable stored app → %d, want 200", status)
	}

	if _, err := apptest.EarlyPool(t).Exec(context.Background(), `DELETE FROM vault_secret`); err != nil {
		t.Fatalf("dropping the sealed secret: %v", err)
	}

	status := e.Call(t, "POST", "/v1/connectors/gmail/connect", nil, nil, nil)
	if status == http.StatusNotImplemented {
		t.Error("connect answered 501 for an app that IS stored but cannot be opened — that sends an operator to create a second app instead of repairing the vault")
	}
	if status == http.StatusOK {
		t.Error("connect answered 200 with a secret it could not unseal; the consent URL would carry a client id whose secret no exchange can use")
	}

	// And the installation still reports the app as configured: it IS, and
	// saying otherwise would invite an operator to store a duplicate rather than
	// fix what is wrong.
	var app crmcontracts.ConnectorApp
	if s := e.Call(t, "GET", "/v1/installation/oauth-apps/google", nil, nil, &app); s != http.StatusOK || !app.Configured {
		t.Errorf("after the secret was lost the app reads %+v (status %d), want still configured", app, s)
	}
}

// The steps arrive in the order onboarding walks them: AI models, then the
// Google app.
//
// The contract calls `steps` "every setup step, in the order a reader should
// complete them", so the sequence is the SERVER's to decide and a client that
// sorted them itself would be re-deciding it. Pinned because the order is a
// product decision rather than an accident of how the slice was typed: a contact
// who has bound no model cannot be shown a cold start at all, while the Gmail
// step is the one they can leave until later. Swapping the two would walk them
// into configuring a mailbox for a product that cannot yet think.
func TestTheSetupReportListsTheStepsInTheOrderOnboardingWalksThem(t *testing.T) {
	e := setupConnectorAppHTTP(t)
	var setup crmcontracts.InstallationSetup
	if status := e.Call(t, "GET", "/v1/installation/setup", nil, nil, &setup); status != http.StatusOK {
		t.Fatalf("GET installation/setup → %d, want 200", status)
	}
	want := []crmcontracts.InstallationSetupStepStep{
		crmcontracts.InstallationSetupStepStepAiModels,
		crmcontracts.InstallationSetupStepStepOauthApp,
	}
	if len(setup.Steps) != len(want) {
		t.Fatalf("the report carries %d steps, want %d: %+v", len(setup.Steps), len(want), setup.Steps)
	}
	for i, step := range want {
		if setup.Steps[i].Step != step {
			t.Errorf("step %d is %q, want %q — onboarding walks this slice in order", i, setup.Steps[i].Step, step)
		}
	}
}

// The Microsoft half of the same surface, which is the whole reason it is keyed
// by vendor: a second copy of the store and the screen would have drifted on
// exactly the field only one vendor has.
func TestMicrosoftAppOverHTTPIsItsOwnAppAndCarriesADirectory(t *testing.T) {
	e := setupConnectorAppHTTP(t)
	const entraID = "11111111-2222-3333-4444-555555555555"
	secret, directory := "entra-over-the-wire", "99999999-8888-7777-6666-555555555555"

	body := crmcontracts.ConnectorAppInput{
		ClientId: entraID, ClientSecret: &secret, Tenant: &directory,
	}
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/microsoft", body, nil, nil); status != http.StatusNoContent {
		t.Fatalf("PUT the Microsoft app → %d, want 204", status)
	}

	var app crmcontracts.ConnectorApp
	if status := e.Call(t, "GET", "/v1/installation/oauth-apps/microsoft", nil, nil, &app); status != http.StatusOK {
		t.Fatalf("GET the Microsoft app → %d, want 200", status)
	}
	if !app.Configured || app.ClientId != entraID {
		t.Fatalf("the Microsoft app reads %+v, want configured with the Entra client id", app)
	}
	if app.Tenant == nil || *app.Tenant != directory {
		t.Fatalf("tenant = %v, want the directory it was pinned to", app.Tenant)
	}

	// Storing one vendor's app must not touch the other's. One settings key for
	// both would have made this write blank Google.
	var google crmcontracts.ConnectorApp
	if status := e.Call(t, "GET", "/v1/installation/oauth-apps/google", nil, nil, &google); status != http.StatusOK {
		t.Fatalf("GET the Google app → %d, want 200", status)
	}
	if google.Configured {
		t.Fatalf("storing the Microsoft app left Google reading %+v", google)
	}
}

// The refusals only Microsoft has. Each is a value that would read as configured
// while authorizing a population nobody vetted, or nothing at all.
func TestMicrosoftAppOverHTTPRefusesAnAliasAndAForeignClientID(t *testing.T) {
	e := setupConnectorAppHTTP(t)
	secret := "entra-over-the-wire"
	alias := "common"
	directory := "99999999-8888-7777-6666-555555555555"
	const entraID = "11111111-2222-3333-4444-555555555555"

	for _, tc := range []struct {
		name string
		body crmcontracts.ConnectorAppInput
	}{
		{"an authority alias where a directory belongs", crmcontracts.ConnectorAppInput{
			ClientId: entraID, ClientSecret: &secret, Tenant: &alias,
		}},
		{"a Google client id on the Microsoft app", crmcontracts.ConnectorAppInput{
			ClientId: httpClientID, ClientSecret: &secret,
		}},
		{"the app's display name", crmcontracts.ConnectorAppInput{
			ClientId: "Margince CRM", ClientSecret: &secret,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/microsoft", tc.body, nil, nil); status != http.StatusUnprocessableEntity {
				t.Fatalf("PUT %s → %d, want 422", tc.name, status)
			}
		})
	}

	// And a directory on a vendor that has none: a field that silently did
	// nothing would have an operator believe they narrowed something.
	google := crmcontracts.ConnectorAppInput{
		ClientId: httpClientID, ClientSecret: &secret, Tenant: &directory,
	}
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/google", google, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("PUT a Google app carrying a tenant → %d, want 422", status)
	}

	// A 422 is only half the claim. The other half is that NOTHING landed:
	// a validator that answered 422 after the write would pass every case above
	// while leaving an installation configured against an app it refused, which
	// is the worse failure of the two — it reads as working until a consent
	// screen says otherwise.
	for _, vendor := range []string{"microsoft", "google"} {
		var app crmcontracts.ConnectorApp
		if status := e.Call(t, "GET", "/v1/installation/oauth-apps/"+vendor, nil, nil, &app); status != http.StatusOK {
			t.Fatalf("GET the %s app → %d, want 200", vendor, status)
		}
		if app.Configured {
			t.Errorf("a refused %s app was stored anyway: %+v", vendor, app)
		}
	}
}

// A vendor this build does not serve is 404 and never a default: defaulting
// would seal somebody's Entra secret under the Google key, where the Google
// connector would present it and the Microsoft one would never find it.
func TestAnUnknownVendorIsNotFoundRatherThanDefaulted(t *testing.T) {
	e := setupConnectorAppHTTP(t)
	secret := "never-sealed"
	for _, verb := range []string{"GET", "DELETE"} {
		if status := e.Call(t, verb, "/v1/installation/oauth-apps/apple", nil, nil, nil); status != http.StatusNotFound {
			t.Errorf("%s an unknown vendor → %d, want 404", verb, status)
		}
	}
	body := crmcontracts.ConnectorAppInput{ClientId: httpClientID, ClientSecret: &secret}
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/apple", body, nil, nil); status != http.StatusNotFound {
		t.Errorf("PUT an unknown vendor → %d, want 404", status)
	}
	// Nothing was stored under either vendor it might have been mistaken for.
	//
	// The STATUS is asserted beside the flag: apptest.Call decodes only a
	// non-empty body, so an error response would leave `app` zero-valued and
	// this pass vacuously — the guard reporting clean having read nothing.
	for _, vendor := range []string{"google", "microsoft"} {
		var app crmcontracts.ConnectorApp
		if status := e.Call(t, "GET", "/v1/installation/oauth-apps/"+vendor, nil, nil, &app); status != http.StatusOK {
			t.Fatalf("GET %s → %d, want 200 — an unread body would make the check below prove nothing", vendor, status)
		}
		if app.Configured {
			t.Errorf("a write to an unknown vendor landed under %s", vendor)
		}
	}
}

// A client id the environment sets without its secret composes no app. So it
// locks nothing, and the stored app stays the admin's to write and remove.
func TestAnEnvClientIDWithoutItsSecretLeavesTheStoredAppWritable(t *testing.T) {
	e, _ := apptest.SetupGoogleConnect(t, "env-only.apps.googleusercontent.com", "")
	secret := httpSecret
	if status := e.Call(t, "PUT", "/v1/installation/oauth-apps/google",
		crmcontracts.ConnectorAppInput{ClientId: httpClientID, ClientSecret: &secret}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("PUT beside a half environment app → %d, want 204", status)
	}
	var app crmcontracts.ConnectorApp
	if status := e.Call(t, "GET", "/v1/installation/oauth-apps/google", nil, nil, &app); status != http.StatusOK ||
		app.ClientId != httpClientID || app.Source != crmcontracts.ConnectorAppSourceStored {
		t.Fatalf("GET → %d %+v, want the stored app in use", status, app)
	}
	if _, ok := appLock(app, capture.GoogleAppKey); ok {
		t.Errorf("locks = %+v, want the stored app unlocked", app.Locks)
	}
	if status := e.Call(t, "DELETE", "/v1/installation/oauth-apps/google", nil, nil, nil); status != http.StatusNoContent {
		t.Errorf("DELETE beside a half environment app → %d, want 204", status)
	}
}

// appLock is the lock a connector app read reports on key.
func appLock(app crmcontracts.ConnectorApp, key string) (crmcontracts.SettingLock, bool) {
	if app.Locks == nil {
		return crmcontracts.SettingLock{}, false
	}
	for _, lock := range *app.Locks {
		if lock.Key == key {
			return lock, true
		}
	}
	return crmcontracts.SettingLock{}, false
}
