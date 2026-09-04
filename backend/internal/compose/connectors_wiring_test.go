// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fakeVault is a non-nil keyvault.Vault for wiring tests; WithGmailCapture only
// checks it's present, never calls it.
type fakeVault struct{}

func (fakeVault) Put(context.Context, ids.WorkspaceID, []byte) (keyvault.Ref, error) {
	return "", nil
}

func (fakeVault) Get(context.Context, ids.WorkspaceID, keyvault.Ref) ([]byte, error) {
	return nil, nil
}

func (fakeVault) GetOn(context.Context, keyvault.Querier, ids.WorkspaceID, keyvault.Ref) ([]byte, error) {
	return nil, nil
}
func (fakeVault) Delete(context.Context, ids.WorkspaceID, keyvault.Ref) error { return nil }
func (fakeVault) Health(context.Context) error                                { return nil }

// recordingOAuth notes whether the token exchange ran, so a CSRF test can tell
// "blocked before the exchange" from "passed the gate and reached it".
type recordingOAuth struct{ exchanged bool }

func (o *recordingOAuth) AuthCodeURL(state, _ string) string { return "https://auth?state=" + state }

func (o *recordingOAuth) Exchange(context.Context, string, string) (oauthflow.TokenGrant, error) {
	o.exchanged = true
	return oauthflow.TokenGrant{RefreshToken: "refresh"}, nil
}

func (o *recordingOAuth) AccessToken(context.Context, string) (string, error) { return "access", nil }

type stubGmailAPI struct{}

func (stubGmailAPI) EstimateAfter(context.Context, string, string) (int, error) { return 0, nil }

func (stubGmailAPI) ListAfter(context.Context, string, string, string, int) ([]string, string, error) {
	return nil, "", nil
}

func (stubGmailAPI) Profile(context.Context, string) (string, string, error) {
	return "owner@example.com", "1", nil
}
func (stubGmailAPI) ListRecent(context.Context, string, int) ([]string, error) { return nil, nil }
func (stubGmailAPI) History(context.Context, string, string) ([]string, string, error) {
	return nil, "1", nil
}

func (stubGmailAPI) GetRaw(context.Context, string, string) (gmail.Message, error) {
	return gmail.Message{}, nil
}

func (stubGmailAPI) Watch(context.Context, string, string) (string, time.Time, error) {
	return "1", time.Time{}, nil
}

func (stubGmailAPI) Send(context.Context, string, string) (string, error) {
	return "", nil
}

func (stubGmailAPI) FindByMessageID(context.Context, string, string) (string, bool, error) {
	return "", false, nil
}

// The account-linking-CSRF defence: the callback must have the provider's
// nonce cookie matching the nonce in the signed state before it exchanges the
// code.
func TestCallbackRequiresMatchingCSRFCookie(t *testing.T) {
	signer := newStateSigner([]byte("0123456789abcdef0123456789abcdef"))
	oauth := &recordingOAuth{}
	h := connectorHandlers{
		registry:      capture.NewRegistry(nil, nil, liveAuthority{}, nil),
		authority:     liveAuthority{},
		oauth:         oauth,
		gmailAPI:      stubGmailAPI{},
		signer:        signer,
		publicBaseURL: "https://app.test",
		apiBaseURL:    "https://api.test",
	}
	const nonce = "csrf-nonce-value"
	state := signer.sign(connectState{
		Workspace: ids.MustParse("11111111-1111-1111-1111-111111111111"),
		User:      ids.MustParse("22222222-2222-2222-2222-222222222222"),
		Provider:  "gmail",
		Nonce:     nonce,
		Version:   stateVersionNamespacedCSRF,
	}, time.Now().Add(time.Hour))
	code := "the-code"
	params := crmcontracts.ConnectorOAuthCallbackParams{Code: &code, State: state}

	// (a) No cookie → blocked before any token exchange.
	rec := httptest.NewRecorder()
	h.ConnectorOAuthCallback(rec, httptest.NewRequest(http.MethodGet, "/cb", nil), "gmail", params)
	if oauth.exchanged {
		t.Fatal("token exchange ran without a matching nonce cookie (CSRF gate bypassed)")
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("no-cookie status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://app.test/#/onboarding/connect/error/gmail" {
		t.Errorf("no-cookie Location = %q, want the error landing", loc)
	}

	// (b) Matching cookie → passes the gate and reaches the exchange.
	oauth.exchanged = false
	req := httptest.NewRequest(http.MethodGet, "/cb", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName("gmail"), Value: nonce, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	h.ConnectorOAuthCallback(httptest.NewRecorder(), req, "gmail", params)
	if !oauth.exchanged {
		t.Fatal("a matching nonce cookie should let the flow reach the token exchange")
	}

}

func TestGmailConfigGating(t *testing.T) {
	full := GmailConfig{ClientID: "id", ClientSecret: "sec", StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://app"}
	cases := []struct {
		name             string
		cfg              GmailConfig
		canSync, connect bool
	}{
		{"full", full, true, true},
		{"sync only (no state/url)", GmailConfig{ClientID: "id", ClientSecret: "sec"}, true, false},
		{"missing secret", GmailConfig{ClientID: "id"}, false, false},
		{"empty", GmailConfig{}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.cfg.canSync() != c.canSync {
				t.Errorf("canSync = %v, want %v", c.cfg.canSync(), c.canSync)
			}
			if c.cfg.canConnect() != c.connect {
				t.Errorf("canConnect = %v, want %v", c.cfg.canConnect(), c.connect)
			}
		})
	}
}

// The stored-app resolver SURVIVES the option order cmd/api uses.
//
// It did not. WithKeyvault installed it and WithGmailCapture then replaced the
// whole connectorHandlers struct with a composite literal that omitted the
// field, so every composed installation fell back to the environment and the
// feature was inert — while every test still passed, because none of them
// applied both options in the order the api does.
//
// Asserted through the applied options rather than by reading the field on a
// hand-built struct: the defect was in the WIRING, and a fixture that builds the
// struct itself cannot see it.
func TestTheStoredAppResolverSurvivesBothCaptureOptions(t *testing.T) {
	var s Server
	// The order cmd/api applies them in: keyvault first, gmail after.
	WithKeyvault(fakeVault{})(&s, nil)
	if s.googleAppResolver == nil {
		t.Fatal("WithKeyvault installed no stored-app resolver")
	}
	if s.googleCredentials == nil {
		t.Error("WithKeyvault did not carry the resolver into the connector handlers")
	}

	WithGmailCapture(GmailConfig{
		ClientID: "id", ClientSecret: "sec",
		StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://app",
	}, CaptureConfig{})(&s, nil)
	if s.googleCredentials == nil {
		t.Error("WithGmailCapture dropped the stored-app resolver; an app set in Settings is unreachable and the installation silently keeps using the environment's")
	}
}

func TestWithGmailCaptureWiresOrSkips(t *testing.T) {
	full := GmailConfig{ClientID: "id", ClientSecret: "sec", StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://app"}

	// Fully configured + a vault → the connector transport is wired.
	var s Server
	s.vault = fakeVault{}
	WithGmailCapture(full, CaptureConfig{})(&s, nil)
	if !s.wired() {
		t.Error("WithGmailCapture(full) with a vault did not wire the connector handlers")
	}
	// The one Google app mounts BOTH connectors: gcal must resolve through the
	// production wiring, not only through the manually-injected route tests.
	// A background context, because the app is now resolved per call. This
	// wiring composes no vault, so there is no resolver and both fall back to
	// the environment-composed app — the case this asserts.
	for _, provider := range []string{providerGmail, providerGcal} {
		app, ok, err := s.oauthApp(context.Background(), provider)
		if err != nil {
			t.Errorf("resolving the %s OAuth app: %v", provider, err)
		}
		if !ok {
			t.Errorf("WithGmailCapture(full) did not compose the %s OAuth app", provider)
		}
		if app.authCodeURL == nil {
			t.Errorf("the %s app has no consent-URL builder", provider)
		}
	}

	// Fully configured but NO vault → no-op (can't seal the refresh token).
	var s2 Server
	WithGmailCapture(full, CaptureConfig{})(&s2, nil)
	if s2.wired() {
		t.Error("WithGmailCapture without a vault should be a no-op")
	}

	// Missing the state key → no-op, surface stays its 501.
	var s3 Server
	s3.vault = fakeVault{}
	WithGmailCapture(GmailConfig{ClientID: "id", ClientSecret: "sec"}, CaptureConfig{})(&s3, nil)
	if s3.wired() {
		t.Error("WithGmailCapture without a state key/base URL should be a no-op")
	}
}

func TestContractMapping(t *testing.T) {
	// Storage now uses the contract's own status vocabulary (CAP-DDL-2), so
	// status is a straight cast — no translation. Cursor + watch deadline surface.
	id := ids.MustParse("11111111-1111-1111-1111-111111111111")
	watch := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	c := toContractConnection(capture.ConnectionView{
		ID: id, Provider: "gmail", Status: "connected",
		Cursor: []byte(`{"history_id":"7"}`), WatchExpiresAt: &watch, ProviderScopes: []string{
			"https://www.googleapis.com/auth/gmail.readonly",
			"https://www.googleapis.com/auth/gmail.send",
		},
	})
	if c.Provider != "gmail" || c.Status != crmcontracts.CaptureConnectionStatusConnected || c.SyncCursor == nil || *c.SyncCursor != `{"history_id":"7"}` {
		t.Errorf("mapping wrong: %+v", c)
	}
	if c.WatchExpiresAt == nil || !c.WatchExpiresAt.Equal(watch) {
		t.Errorf("watch_expires_at not surfaced: %+v", c.WatchExpiresAt)
	}
	// The reauth_required status also passes straight through.
	if got := toContractConnection(capture.ConnectionView{Provider: "gmail", Status: "reauth_required"}); got.Status != crmcontracts.CaptureConnectionStatusReauthRequired {
		t.Errorf("reauth_required → %q, want reauth_required", got.Status)
	}
	// The wire `scopes` is the PROVIDER's vocabulary, not the internal one —
	// and it freezes what Google actually granted, read + send both.
	wantScopes := []string{
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/gmail.send",
	}
	if !slices.Equal(c.Scopes, wantScopes) {
		t.Errorf("scopes = %v, want %v", c.Scopes, wantScopes)
	}
	// A connection whose grant was never recorded maps to an empty slice,
	// never null.
	if c2 := toContractConnection(capture.ConnectionView{Provider: "gmail", Status: "connected"}); c2.Scopes == nil {
		t.Error("nil scopes should map to an empty slice")
	}
}

func TestListAndDisconnectNotImplementedWhenUnwired(t *testing.T) {
	var h connectorHandlers // no registry
	for _, tc := range []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
	}{
		{"list", h.ListConnectors},
		{"disconnect", func(w http.ResponseWriter, r *http.Request) { h.DisconnectConnector(w, r, "gmail") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.call(rec, httptest.NewRequest(http.MethodGet, "/v1/connectors", nil))
			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("%s unwired = %d, want 501", tc.name, rec.Code)
			}
		})
	}
}

// A stored-app installation — no Google app in the ENVIRONMENT, the deployment's
// own state key and callback base present — mounts the transport, registers the
// Google connectors off the stored-app resolver, and must still refuse the
// connect verb until an app is actually stored. NOTHING here is unbuilt: the
// resolver is wired, gmail and gcal are registered, and the only thing this
// composition lacks is a row in the store. The refusal is `app_missing`, which
// is what an empty store means.
//
// The refusal is the point. Before this was gated, the transport mounted and
// gmailApp fell back to the env-composed client, which existed with an EMPTY
// client id: the caller was redirected to Google's consent screen with
// `client_id=` and the flow failed THERE, after they had already been sent away,
// with nothing they could act on. A 501 at the gate is the honest answer to a
// vendor this installation has not registered an app for.
func TestAStoredAppInstallationRefusesConnectRatherThanSigningBlankCredentials(t *testing.T) {
	var s Server
	// cmd/api's order, and the stored-app shape: a vault to seal into, no
	// MARGINCE_GMAIL_* in the environment, deployment prerequisites present.
	WithKeyvault(fakeVault{})(&s, nil)
	WithGmailCapture(GmailConfig{
		StateKey:      "0123456789abcdef0123456789abcdef",
		PublicBaseURL: "https://app.example",
	}, CaptureConfig{})(&s, nil)

	// Mounted: the registry is what the transport serves IMAP from, and IMAP
	// needs no Google app at all — so this must not become an unmounted surface.
	// (`wired()` is deliberately not the check: it also requires the
	// env-composed Google client, which is exactly what a stored-app
	// installation does not have.)
	if s.connectorHandlers.registry == nil {
		t.Fatal("the transport must mount on the deployment's own prerequisites: the app can arrive at runtime")
	}
	for _, provider := range []crmcontracts.CaptureProvider{
		crmcontracts.CaptureProviderGmail, crmcontracts.CaptureProviderGcal,
	} {
		t.Run(string(provider), func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.ConnectConnector(
				rec, httptest.NewRequest(http.MethodPost, "/v1/connectors/"+string(provider)+"/connect", nil), provider)
			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("connect on a stored-app installation = %d, want 501: anything else means the request got PAST the gate that should have refused it, "+
					"and on a real request with a signed-in human that is a redirect to Google carrying an empty client id", rec.Code)
			}
		})
	}
}
