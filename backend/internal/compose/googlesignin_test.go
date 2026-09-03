// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
)

func TestGoogleSignInConfigEnabled(t *testing.T) {
	complete := GoogleSignInConfig{
		ClientID: "cid", ClientSecret: "secret", StateKey: "0123456789012345678901234567890123",
		RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	}
	if !complete.Enabled() {
		t.Fatal("expected Enabled() true for a fully configured GoogleSignInConfig")
	}

	incomplete := complete
	incomplete.StateKey = ""
	if incomplete.Enabled() {
		t.Fatal("expected Enabled() false when the state key is missing")
	}
	if got := incomplete.MissingFields(); len(got) != 1 || got[0] != "state key (>=32B)" {
		t.Fatalf("MissingFields() = %v", got)
	}
}

func TestWithGoogleSignInAbsentIsNoOp(t *testing.T) {
	opt := WithGoogleSignIn(GoogleSignInConfig{})
	if opt == nil {
		t.Fatal("WithGoogleSignIn must always return a valid Option, even unconfigured")
	}
	var s Server
	opt(&s, nil) // must not panic on a zero config

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil)
	rec := httptest.NewRecorder()
	s.GetAuthCapabilities(rec, req)
	var body struct {
		OidcProviders []struct{ Key string } `json:"oidc_providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.OidcProviders) != 0 {
		t.Fatalf("an unconfigured GoogleSignInConfig must not report any oidc_providers, got %+v", body.OidcProviders)
	}
}

func TestWithGoogleSignInCompleteMountsAndReportsCapability(t *testing.T) {
	opt := WithGoogleSignIn(GoogleSignInConfig{
		ClientID: "cid", ClientSecret: "secret", StateKey: "0123456789012345678901234567890123",
		RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	})
	var s Server
	opt(&s, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil)
	rec := httptest.NewRecorder()
	s.GetAuthCapabilities(rec, req)
	var body struct {
		OidcProviders []struct{ Key, Label string } `json:"oidc_providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.OidcProviders) != 1 || body.OidcProviders[0].Key != "google" {
		t.Fatalf("oidc_providers = %+v, want one entry keyed google", body.OidcProviders)
	}

	startReq := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/start", nil)
	startRec := httptest.NewRecorder()
	s.StartOidcSignIn(startRec, startReq, "google")
	if startRec.Code != http.StatusFound {
		t.Fatalf("StartOidcSignIn status = %d, want 302 (the route the capability just advertised must actually be mounted)", startRec.Code)
	}
}

// The audience check is shared by every provider — a token minted for another
// app's client id says nothing about who may sign in here, whoever issued it.
func TestTheAudienceCheckRefusesAnotherAppsToken(t *testing.T) {
	match := audienceIs("cid")
	if err := match(oidcClaims{Aud: "cid"}); err != nil {
		t.Fatalf("match(correct aud) = %v, want nil", err)
	}
	if err := match(oidcClaims{Aud: "someone-elses-client"}); err == nil {
		t.Fatal("expected a mismatch to be rejected")
	}
	if err := match(oidcClaims{}); err == nil {
		t.Fatal("a token naming no audience at all was accepted")
	}
}

func TestGoogleOIDCVerifierAdapterTranslatesClaims(t *testing.T) {
	rig := newOIDCTestRig(t)
	v := newGoogleOIDCVerifier(rig.jwksURL(), identityResolvedPerRequest).
		withHTTPClient(rig.srv.Client()).
		withClock(func() time.Time { return rig.base })
	adapter := googleOIDCVerifierAdapter{v: v, matchIdentity: func(oidcClaims) error { return nil }}

	tok := rig.mint(t, testKID, "RS256", map[string]any{"sub": "sub-1", "email": "carol@example.com"})
	email, sub, verified, err := adapter.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if email != "carol@example.com" || sub != "sub-1" || !verified {
		t.Fatalf("email=%q sub=%q verified=%v", email, sub, verified)
	}

	if _, _, _, err := adapter.Verify(context.Background(), "not-a-jwt"); err == nil {
		t.Fatal("expected an error to pass through for a malformed token")
	}
}

func TestGoogleTokenExchangerAdapterDelegates(t *testing.T) {
	var gotForm url.Values
	var gotContentType string
	var gotParseErr error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotParseErr = r.ParseForm()
		gotForm = r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id_token":"the.id.token"}`))
	}))
	defer srv.Close()

	adapter := oidcExchangerAdapter{ex: oidcCodeExchanger{ClientID: "cid", ClientSecret: "secret", TokenURL: srv.URL, HTTPClient: srv.Client()}}
	idToken, err := adapter.Exchange(context.Background(), "code", "verifier", "https://app.example.com/cb")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if gotParseErr != nil {
		t.Fatalf("server failed to parse the exchange request's form body: %v", gotParseErr)
	}
	if idToken != "the.id.token" {
		t.Fatalf("idToken = %q", idToken)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
	if gotForm.Get("code") != "code" || gotForm.Get("code_verifier") != "verifier" ||
		gotForm.Get("redirect_uri") != "https://app.example.com/cb" || gotForm.Get("client_id") != "cid" {
		t.Fatalf("form = %v, want code/code_verifier/redirect_uri/client_id passed through", gotForm)
	}
}

func TestLoginStateSignerAdapterRoundTrips(t *testing.T) {
	adapter := loginStateSignerAdapter{s: newLoginStateSigner([]byte("0123456789012345678901234567890123"))}
	token := adapter.Sign("google", "nonce-1", "verifier-1", 10*time.Minute)

	provider, nonce, codeVerifier, err := adapter.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if provider != "google" || nonce != "nonce-1" || codeVerifier != "verifier-1" {
		t.Fatalf("provider=%q nonce=%q codeVerifier=%q", provider, nonce, codeVerifier)
	}

	if _, _, _, err := adapter.Verify("not-a-real-token"); err == nil {
		t.Fatal("expected an error to pass through for a malformed token")
	}
}

// The intersection is the whole rule of the provider policy: an admin narrows
// what the deployment composed and can never widen it, because a key nobody
// holds credentials for cannot be made to work by choosing it.
func TestOfferedProvidersOnlyEverNarrowsWhatTheDeploymentComposed(t *testing.T) {
	google := identity.OIDCProviderConfig{Key: "google", Label: "Google"}
	microsoft := identity.OIDCProviderConfig{Key: "microsoft", Label: "Microsoft"}
	configured := []identity.OIDCProviderConfig{google, microsoft}

	for name, tc := range map[string]struct {
		chosen []string
		want   []string
	}{
		// Never chosen is not the same as chose-none: an installation that
		// upgrades into this setting keeps the login screen it had, rather than
		// silently losing every provider on the deploy that introduced it.
		"never chosen offers everything configured": {chosen: nil, want: []string{"google", "microsoft"}},
		"an empty choice offers nothing":            {chosen: []string{}, want: nil},
		"a choice narrows to itself":                {chosen: []string{"google"}, want: []string{"google"}},
		// The reason this is an intersection rather than a lookup: an operator
		// cannot invent a client id from a settings screen, so a key the
		// deployment holds no credentials for must enable nothing at all.
		"a key the deployment never composed enables nothing": {chosen: []string{"okta"}, want: nil},
		"an unknown key alongside a real one narrows to the real one": {
			chosen: []string{"okta", "microsoft"}, want: []string{"microsoft"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var got []string
			for _, p := range offeredProviders(configured, tc.chosen) {
				got = append(got, p.Key)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("offered %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("offered %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// The client a flow runs on is resolved when the flow runs, the stored app
// first: the app an admin saves during the first run reaches the login screen
// without a restart, and the environment's pair is what a deployment without
// one falls back to. A vault that will not open is neither — it is reported,
// never quietly replaced by the older environment copy.
func TestGoogleSignInRunsOnTheStoredAppBeforeTheEnvironmentsPair(t *testing.T) {
	ctx := context.Background()
	env := signInClient{ClientID: "env-cid", ClientSecret: "env-secret"}
	verifier := newGoogleOIDCVerifier(googleJWKSURL, identityResolvedPerRequest)
	stored := func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{ClientID: "stored-cid", ClientSecretRef: "stored-secret"}, true, nil
	}
	none := func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{}, false, nil
	}
	sealed := func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{}, false, errors.New("the vault root key moved")
	}

	p, ok, err := googleSignInSource{env: env, stored: stored, verifier: verifier}.provider(ctx)
	if err != nil || !ok {
		t.Fatalf("with a stored app: ok=%v err=%v", ok, err)
	}
	if p.Config.ClientID != "stored-cid" {
		t.Errorf("client id = %q, want the stored app's", p.Config.ClientID)
	}
	// The exchanger redeems on the SAME client the browser was sent out with,
	// or the token endpoint refuses a code minted for another app.
	if ex, isAdapter := p.Exchanger.(oidcExchangerAdapter); !isAdapter || ex.ex.ClientSecret != "stored-secret" {
		t.Errorf("exchanger = %#v, want the stored app's secret", p.Exchanger)
	}

	p, ok, err = googleSignInSource{env: env, stored: none, verifier: verifier}.provider(ctx)
	if err != nil || !ok || p.Config.ClientID != "env-cid" {
		t.Fatalf("without a stored app: ok=%v err=%v client=%q, want the environment's pair", ok, err, p.Config.ClientID)
	}

	if _, ok, err := (googleSignInSource{stored: none, verifier: verifier}).provider(ctx); ok || err != nil {
		t.Fatalf("with no client from any source: ok=%v err=%v, want withheld without error", ok, err)
	}

	if _, _, err := (googleSignInSource{env: env, stored: sealed, verifier: verifier}).provider(ctx); err == nil {
		t.Fatal("a secret that will not open fell back to the environment's pair, hiding a moved vault key")
	}
}

// A deployment with the state key and the URLs but no pair mounts the routes
// and offers nothing: the button appears the moment a client exists, and until
// then the start route answers as for an absent provider rather than sending a
// browser out with an empty client id. The settings screen still lists the
// provider, because the app card beside it is where it is made to work.
func TestGoogleSignInMountedWithoutAClientIsNotOffered(t *testing.T) {
	var s Server
	WithGoogleSignIn(GoogleSignInConfig{
		StateKey:     "0123456789012345678901234567890123",
		RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	})(&s, nil)

	if got := oidcProviderKeys(t, &s); len(got) != 0 {
		t.Fatalf("oidc_providers = %v, want none while no client exists", got)
	}
	rec := httptest.NewRecorder()
	s.StartOidcSignIn(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/start", nil), "google")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("StartOidcSignIn = %d, want 404 for a mounted provider with no client", rec.Code)
	}
	if len(s.configuredProviders) != 1 || s.configuredProviders[0].Key != "google" {
		t.Fatalf("configuredProviders = %+v, want google listed for the settings screen", s.configuredProviders)
	}
}
