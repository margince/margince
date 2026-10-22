// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A unit's OAuth app end to end without a database. The consent flow runs
// against an in-test vendor; the grant and the token are read from memory.
// The sealed storage is extsecrets', proven there.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/pkg/extension"
)

var oauthEpoch = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// workspaceSecrets is the core-held port in memory, workspace scope only.
type workspaceSecrets struct {
	extension.Secrets
	held map[string][]byte
}

func (s *workspaceSecrets) Get(_ context.Context, key string) ([]byte, error) {
	v, ok := s.held[key]
	if !ok {
		return nil, extension.ErrSecretNotFound
	}
	return v, nil
}

func (s *workspaceSecrets) Put(_ context.Context, key string, secret []byte) error {
	s.held[key] = secret
	return nil
}

func (s *workspaceSecrets) Delete(_ context.Context, key string) error {
	if _, ok := s.held[key]; !ok {
		return extension.ErrSecretNotFound
	}
	delete(s.held, key)
	return nil
}

// vendor is the in-test token endpoint: it answers each grant type with the
// scripted status and body, and records the forms it received.
type vendor struct {
	exchange, refresh string
	status            int
	forms             []url.Values
}

func (v *vendor) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		v.forms = append(v.forms, r.PostForm)
		if v.status != 0 {
			w.WriteHeader(v.status)
		}
		body := v.refresh
		if r.PostForm.Get("grant_type") == "authorization_code" {
			body = v.exchange
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func booksApp(tokenURL string) extension.OAuthApp {
	return extension.OAuthApp{
		Provider: "books", Title: "Books ledger",
		AuthURL: "https://auth.ledgerly.test/authorize", TokenURL: tokenURL,
		Scopes: []string{"ledger.read"}, AuthParams: map[string]string{"prompt": "consent"},
	}
}

// testGrants is one unit's grants over in-memory secrets, a stored app the
// test decides, and a fixed clock.
func testGrants(stored bool) (extOAuthGrants, *workspaceSecrets) {
	secrets := &workspaceSecrets{held: map[string][]byte{}}
	return extOAuthGrants{
		secrets: secrets,
		credentials: func(context.Context, string) (capture.ConnectorApp, bool, error) {
			if !stored {
				return capture.ConnectorApp{}, false, nil
			}
			return capture.ConnectorApp{ClientID: "ledgerly-client", ClientSecretRef: "ledgerly-secret"}, true, nil
		},
		now: func() time.Time { return oauthEpoch },
	}, secrets
}

func heldGrant(t *testing.T, secrets *workspaceSecrets) storedOAuthGrant {
	t.Helper()
	var grant storedOAuthGrant
	if err := json.Unmarshal(secrets.held[grantKey("books")], &grant); err != nil {
		t.Fatalf("the held grant does not decode: %v", err)
	}
	return grant
}

func TestAFreshAccessTokenIsServedWithoutAskingTheVendor(t *testing.T) {
	v := &vendor{}
	grants, _ := testGrants(true)
	if err := grants.save(context.Background(), "books", storedOAuthGrant{
		RefreshToken: "r1", AccessToken: "a1", ExpiresAt: oauthEpoch.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := grants.accessToken(context.Background(), booksApp(v.serve(t).URL))
	if err != nil || got != "a1" {
		t.Fatalf("accessToken = %q, %v; want the stored a1", got, err)
	}
	if len(v.forms) != 0 {
		t.Fatalf("the vendor was asked %d times for a token that had an hour left", len(v.forms))
	}
}

// Inside the skew counts as expired: a token handed out with seconds left dies
// on its way to the vendor.
func TestAnAccessTokenNearExpiryIsRefreshedAndTheRotationKept(t *testing.T) {
	v := &vendor{refresh: `{"access_token":"a2","refresh_token":"r2","expires_in":3600}`}
	grants, secrets := testGrants(true)
	if err := grants.save(context.Background(), "books", storedOAuthGrant{
		RefreshToken: "r1", AccessToken: "a1", ExpiresAt: oauthEpoch.Add(oauthTokenSkew / 2),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := grants.accessToken(context.Background(), booksApp(v.serve(t).URL))
	if err != nil || got != "a2" {
		t.Fatalf("accessToken = %q, %v; want the refreshed a2", got, err)
	}
	form := v.forms[0]
	if form.Get("refresh_token") != "r1" || form.Get("client_secret") != "ledgerly-secret" {
		t.Fatalf("refresh form = %v, want the stored refresh token under the stored app", form)
	}
	want := storedOAuthGrant{RefreshToken: "r2", AccessToken: "a2", ExpiresAt: oauthEpoch.Add(time.Hour)}
	if got := heldGrant(t, secrets); got != want {
		t.Fatalf("held grant = %+v, want %+v", got, want)
	}
}

func TestARevokedGrantIsForgottenAndReportedAsRevoked(t *testing.T) {
	v := &vendor{status: http.StatusBadRequest, refresh: `{"error":"invalid_grant"}`}
	grants, secrets := testGrants(true)
	if err := grants.save(context.Background(), "books", storedOAuthGrant{RefreshToken: "r1"}); err != nil {
		t.Fatal(err)
	}
	_, err := grants.accessToken(context.Background(), booksApp(v.serve(t).URL))
	if !errors.Is(err, extension.ErrOAuthRevoked) {
		t.Fatalf("accessToken = %v, want ErrOAuthRevoked", err)
	}
	if _, held := secrets.held[grantKey("books")]; held {
		t.Fatal("a grant the vendor refused is still held, so every call asks the vendor again")
	}
	if _, err := grants.accessToken(context.Background(), booksApp("")); !errors.Is(err, extension.ErrOAuthNotConnected) {
		t.Fatalf("the next call = %v, want ErrOAuthNotConnected", err)
	}
}

// A refused CLIENT is the admin's app card to fix, not a revoked grant, so the
// grant stays.
func TestARefusedClientKeepsTheGrant(t *testing.T) {
	v := &vendor{status: http.StatusUnauthorized, refresh: `{"error":"invalid_client"}`}
	grants, secrets := testGrants(true)
	if err := grants.save(context.Background(), "books", storedOAuthGrant{RefreshToken: "r1"}); err != nil {
		t.Fatal(err)
	}
	_, err := grants.accessToken(context.Background(), booksApp(v.serve(t).URL))
	if err == nil || errors.Is(err, extension.ErrOAuthRevoked) {
		t.Fatalf("accessToken = %v, want a refusal that is not a revocation", err)
	}
	if strings.Contains(err.Error(), "r1") {
		t.Fatalf("the refusal %q carries the refresh token", err)
	}
	if _, held := secrets.held[grantKey("books")]; !held {
		t.Fatal("the grant was forgotten over the app's own credentials")
	}
}

func TestATokenForAnUnconnectedOrUnconfiguredAppIsRefused(t *testing.T) {
	grants, _ := testGrants(true)
	if _, err := grants.accessToken(context.Background(), booksApp("")); !errors.Is(err, extension.ErrOAuthNotConnected) {
		t.Fatalf("no grant held: %v, want ErrOAuthNotConnected", err)
	}
	unconfigured, _ := testGrants(false)
	if err := unconfigured.save(context.Background(), "books", storedOAuthGrant{RefreshToken: "r1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := unconfigured.accessToken(context.Background(), booksApp("")); !errors.Is(err, errExtOAuthAppMissing) {
		t.Fatalf("no app stored: %v, want errExtOAuthAppMissing", err)
	}
}

func TestAUnitOAuthAppMayNotTakeACoreNameOrAnotherUnitsProvider(t *testing.T) {
	app := booksApp("https://auth.ledgerly.test/token")
	unit := func(name extension.Name, apps ...extension.OAuthApp) extension.Extension {
		return extension.Extension{Name: name, OAuthApps: apps}
	}
	for _, core := range []string{"google", "microsoft", "gmail", "graphcal", "imap"} {
		shadow := app
		shadow.Provider = core
		if err := preflightOAuthApps(unit("ledger", shadow), map[string]extension.Name{}); err == nil {
			t.Errorf("a unit claiming %q was admitted", core)
		}
	}
	claimed := map[string]extension.Name{}
	if err := preflightOAuthApps(unit("ledger", app), claimed); err != nil {
		t.Fatalf("the first unit's app was refused: %v", err)
	}
	if err := preflightOAuthApps(unit("ledger2", app), claimed); err == nil || !strings.Contains(err.Error(), `"ledger"`) {
		t.Fatalf("a second unit claiming books = %v, want a refusal naming the first", err)
	}
	insecure := app
	insecure.Provider, insecure.TokenURL = "other", "http://auth.ledgerly.test/token"
	if err := preflightOAuthApps(unit("ledger3", insecure), claimed); !errors.Is(err, extension.ErrInvalid) {
		t.Fatalf("an http token endpoint = %v, want the published refusal", err)
	}
}

// The provider a unit app's state carries is outside the provider grammar, so
// no state minted for one verifies at a capture callback.
func TestAUnitAppsStateCannotBeACaptureProvidersState(t *testing.T) {
	for _, p := range append(coreOAuthNames(), "books") {
		if extOAuthStateProvider("books") == p {
			t.Fatalf("a unit app's state provider %q is a provider name", p)
		}
	}
}

func adminRBAC() authz.RBAC {
	return authz.RBAC{Permissions: principal.Permissions{
		Objects: map[string]principal.ObjectGrant{objectIntegrations: {Create: true, Read: true}},
	}}
}

// adminAuthority answers a live admin; repAuthority a live member who may not
// connect an integration.
type adminAuthority struct{ liveAuthority }

func (adminAuthority) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return adminRBAC(), nil
}

func extAdminCtx() context.Context {
	ctx := humanCtx()
	actor, _ := principal.Actor(ctx)
	actor.Permissions = adminRBAC().Permissions
	return principal.WithActor(ctx, actor)
}

// extFlow is a connector transport with one composed unit declaring books
// against the in-test vendor, and the grants it keeps.
type extFlow struct {
	h       connectorHandlers
	secrets *workspaceSecrets
	vendor  *vendor
	granted []extension.OAuthGrant
}

func newExtFlow(t *testing.T, v *vendor, onGrantErr error) *extFlow {
	t.Helper()
	f := &extFlow{vendor: v}
	grants, secrets := testGrants(true)
	f.secrets = secrets
	app := booksApp(v.serve(t).URL)
	app.OnGrant = func(_ context.Context, rt extension.Runtime, grant extension.OAuthGrant) error {
		if rt.Caller().Type != extension.CallerHuman {
			t.Errorf("OnGrant ran as %+v, want the admin who consented", rt.Caller())
		}
		f.granted = append(f.granted, grant)
		return onGrantErr
	}
	setComposedExtensions([]extension.Extension{{Name: "ledger", Version: "0.1.0", OAuthApps: []extension.OAuthApp{app}}})
	t.Cleanup(func() { setComposedExtensions(nil) })
	f.h = connectorHandlers{
		authority:      adminAuthority{},
		signer:         newStateSigner([]byte(testStateKey)),
		publicBaseURL:  "https://app.test",
		apiBaseURL:     "https://api.test",
		extOAuthGrants: func(string) extOAuthGrants { return grants },
	}
	return f
}

// connect drives the connect route and answers the consent URL and the nonce
// cookie it set.
func (f *extFlow) connect(t *testing.T) (*url.URL, *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	f.h.ConnectExtOauth(rec, httptest.NewRequest(http.MethodPost, "/v1/ext-oauth/books/connect", nil).WithContext(extAdminCtx()), "books")
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	return readExtConnect(t, rec)
}

// readExtConnect reads a connect answer: the consent URL and the nonce cookie.
func readExtConnect(t *testing.T, rec *httptest.ResponseRecorder) (*url.URL, *http.Cookie) {
	t.Helper()
	var resp crmcontracts.ExtOAuthConnectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	consent, err := url.Parse(resp.AuthorizeUrl)
	if err != nil {
		t.Fatalf("authorize_url: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("connect set %d cookies, want the one nonce", len(cookies))
	}
	return consent, cookies[0]
}

// callback drives the vendor's redirect back with query and cookie, and answers
// the landing.
func (f *extFlow) callback(t *testing.T, query url.Values, cookie *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/ext-oauth/books/callback?"+query.Encode(), nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	params := crmcontracts.ExtOauthCallbackParams{State: query.Get("state")}
	if code := query.Get("code"); code != "" {
		params.Code = &code
	}
	if denial := query.Get("error"); denial != "" {
		params.Error = &denial
	}
	rec := httptest.NewRecorder()
	f.h.ExtOauthCallback(rec, req, "books", params)
	if rec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want 302 (body %s)", rec.Code, rec.Body)
	}
	return rec.Header().Get("Location")
}

func landing(outcome string) string {
	return "https://app.test/#/ext/ledger?oauth=" + outcome + "&provider=books"
}

func TestAnAdminConnectsAUnitsOAuthAppEndToEnd(t *testing.T) {
	f := newExtFlow(t, &vendor{exchange: `{"access_token":"a1","refresh_token":"r1","expires_in":3600}`}, nil)
	consent, cookie := f.connect(t)

	q := consent.Query()
	if got := q.Get("redirect_uri"); got != "https://api.test/v1/ext-oauth/books/callback" {
		t.Errorf("redirect_uri = %q, want the api's ext-oauth callback", got)
	}
	if q.Get("client_id") != "ledgerly-client" || q.Get("prompt") != "consent" || q.Get("scope") != "ledger.read" {
		t.Errorf("consent query = %v, want the stored client, the unit's scope and its extra parameter", q)
	}
	if cookie.Path != extOAuthPathPrefix || cookie.Name != csrfCookieName("ext-books") {
		t.Errorf("nonce cookie = %s on %s, want it scoped to %s", cookie.Name, cookie.Path, extOAuthPathPrefix)
	}

	got := f.callback(t, url.Values{"code": {"the-code"}, "state": {q.Get("state")}, "realmId": {"company-42"}}, cookie)
	if got != landing(outcomeOK) {
		t.Fatalf("landing = %q, want %q", got, landing(outcomeOK))
	}
	if want := (storedOAuthGrant{RefreshToken: "r1", AccessToken: "a1", ExpiresAt: oauthEpoch.Add(time.Hour)}); heldGrant(t, f.secrets) != want {
		t.Fatalf("held grant = %+v, want %+v", heldGrant(t, f.secrets), want)
	}
	if len(f.granted) != 1 || len(f.granted[0].Params) != 1 || f.granted[0].Params["realmId"] != "company-42" {
		t.Fatalf("OnGrant saw %+v, want only the vendor's realmId", f.granted)
	}
}

func TestAUnitRefusingTheGrantLeavesNoGrantHeld(t *testing.T) {
	f := newExtFlow(t, &vendor{exchange: `{"access_token":"a1","refresh_token":"r1"}`}, errors.New("no company id"))
	consent, cookie := f.connect(t)
	got := f.callback(t, url.Values{"code": {"c"}, "state": {consent.Query().Get("state")}}, cookie)
	if got != landing(outcomeError) {
		t.Fatalf("landing = %q, want %q", got, landing(outcomeError))
	}
	if len(f.secrets.held) != 0 {
		t.Fatal("a grant the unit refused is still held")
	}
}

func TestACallbackThatFailsBeforeTheExchangeLandsOnTheUnitsScreen(t *testing.T) {
	for name, tc := range map[string]struct {
		query      func(state string) url.Values
		withCookie bool
		want       string
	}{
		"the admin declined": {
			func(s string) url.Values { return url.Values{"error": {"access_denied"}, "state": {s}} }, true, outcomeDenied,
		},
		"no nonce cookie": {
			func(s string) url.Values { return url.Values{"code": {"c"}, "state": {s}} }, false, outcomeError,
		},
		"a forged state": {
			func(string) url.Values { return url.Values{"code": {"c"}, "state": {"forged.state"}} }, true, outcomeError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newExtFlow(t, &vendor{exchange: `{"refresh_token":"r1"}`}, nil)
			consent, cookie := f.connect(t)
			if !tc.withCookie {
				cookie = nil
			}
			if got := f.callback(t, tc.query(consent.Query().Get("state")), cookie); got != landing(tc.want) {
				t.Fatalf("landing = %q, want %q", got, landing(tc.want))
			}
			if len(f.vendor.forms) != 0 || len(f.secrets.held) != 0 {
				t.Fatal("the code was exchanged or a grant kept on a callback that never proved itself")
			}
		})
	}
}

// A capture flow's state, signed by the same key, does not complete a unit's.
func TestACaptureStateDoesNotCompleteAUnitAppsFlow(t *testing.T) {
	f := newExtFlow(t, &vendor{exchange: `{"refresh_token":"r1"}`}, nil)
	state := f.h.signer.sign(connectState{
		Workspace: ids.MustParse("11111111-1111-1111-1111-111111111111"),
		User:      ids.MustParse("22222222-2222-2222-2222-222222222222"),
		Provider:  "books", Nonce: "n", Version: stateVersionNamespacedCSRF,
	}, clocktest.Now(t).Add(connectStateTTL))
	cookie := &http.Cookie{Name: csrfCookieName("books"), Value: "n"}
	if got := f.callback(t, url.Values{"code": {"c"}, "state": {state}}, cookie); got != landing(outcomeError) {
		t.Fatalf("landing = %q, want %q", got, landing(outcomeError))
	}
}

func TestAVendorRefusingTheClientLandsOnBadClient(t *testing.T) {
	f := newExtFlow(t, &vendor{status: http.StatusUnauthorized, exchange: `{"error":"invalid_client"}`}, nil)
	consent, cookie := f.connect(t)
	got := f.callback(t, url.Values{"code": {"c"}, "state": {consent.Query().Get("state")}}, cookie)
	if got != landing(outcomeBadClient) {
		t.Fatalf("landing = %q, want %q", got, landing(outcomeBadClient))
	}
}

func TestConnectRefusesANonAdminAnUnknownAppAndAnAppWithNoStoredClient(t *testing.T) {
	f := newExtFlow(t, &vendor{}, nil)
	for name, tc := range map[string]struct {
		ctx      context.Context
		provider string
		grants   func(string) extOAuthGrants
		want     int
	}{
		"a member without the integrations grant": {humanCtx(), "books", nil, http.StatusForbidden},
		"a provider no unit declares":             {extAdminCtx(), "ledgerly", nil, http.StatusNotFound},
		"an app with no stored client": {extAdminCtx(), "books", func(string) extOAuthGrants {
			g, _ := testGrants(false)
			return g
		}, http.StatusNotImplemented},
	} {
		t.Run(name, func(t *testing.T) {
			h := f.h
			if tc.grants != nil {
				h.extOAuthGrants = tc.grants
			}
			rec := httptest.NewRecorder()
			h.ConnectExtOauth(rec, httptest.NewRequest(http.MethodPost, "/v1/ext-oauth/x/connect", nil).WithContext(tc.ctx), tc.provider)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

// A unit reads only its own apps' grants: a neighbour's provider reads as no
// app at all.
func TestAUnitCannotReachAnotherUnitsOAuthGrant(t *testing.T) {
	setComposedExtensions([]extension.Extension{
		{Name: "ledger", Version: "0.1.0", OAuthApps: []extension.OAuthApp{booksApp("https://auth.ledgerly.test/token")}},
		{Name: "neighbour", Version: "0.1.0"},
	})
	t.Cleanup(func() { setComposedExtensions(nil) })
	rt := runtimeFor(extAdminCtx(), "neighbour", "0.1.0", "tool/probe", extensionRuntimeBinding{})
	if _, err := rt.OAuthAccessToken(context.Background(), "books"); !errors.Is(err, extension.ErrNoSuchOAuthApp) {
		t.Fatalf("OAuthAccessToken(books) from a neighbour = %v, want ErrNoSuchOAuthApp", err)
	}
	if err := rt.OAuthDisconnect(context.Background(), "books"); !errors.Is(err, extension.ErrNoSuchOAuthApp) {
		t.Fatalf("OAuthDisconnect(books) from a neighbour = %v, want ErrNoSuchOAuthApp", err)
	}
	own := runtimeFor(extAdminCtx(), "ledger", "0.1.0", "tool/probe", extensionRuntimeBinding{})
	if _, err := own.OAuthAccessToken(context.Background(), "books"); !errors.Is(err, errExtensionRuntimeUnwired) {
		t.Fatalf("the declaring unit = %v, want past the declaration check to the unwired binding", err)
	}
}
