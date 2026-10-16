// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit's OAuth app over real Postgres and the real stores. The client
// credentials sit in the installation app store and the grant in the unit's
// namespace, where the unit's own Secrets port cannot reach it.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/extsecrets"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestAConnectedUnitAppServesItsTokenAndKeepsItFromTheUnit(t *testing.T) {
	e := setupExtRuntime(t)
	v := &vendor{exchange: `{"access_token":"a1","refresh_token":"r1","expires_in":3600}`}
	app := booksApp(v.serve(t).URL)
	app.OnGrant = func(ctx context.Context, rt extension.Runtime, grant extension.OAuthGrant) error {
		return rt.Secrets().Put(ctx, "company", []byte(grant.Params["realmId"]))
	}
	setComposedExtensions([]extension.Extension{{Name: "ledger", Version: "0.1.0", OAuthApps: []extension.OAuthApp{app}}})
	t.Cleanup(func() { setComposedExtensions(nil) })

	system := e.callCtx(e.WS)
	apps := capture.NewConnectorAppStore(NewSettingsStore(e.Pool), e.vault, nil).WithUnitApps(composedUnitApp)
	if err := apps.Set(system, "books", "ledgerly-client", "ledgerly-secret", ""); err != nil {
		t.Fatalf("storing the unit's app: %v", err)
	}

	h := connectorHandlers{
		authority:     adminAuthority{},
		signer:        newStateSigner([]byte(testStateKey)),
		publicBaseURL: "https://app.test", apiBaseURL: "https://api.test",
	}
	admin := e.humanCtx(adminRBAC().Permissions)
	rec := httptest.NewRecorder()
	h.ConnectExtOauth(rec, httptest.NewRequest(http.MethodPost, "/v1/ext-oauth/books/connect", nil).WithContext(admin), "books")
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	f := &extFlow{h: h, vendor: v}
	consent, cookie := readExtConnect(t, rec)
	query := url.Values{"code": {"c"}, "state": {consent.Query().Get("state")}, "realmId": {"company-42"}}
	if got := f.callback(t, query, cookie); got != landing(outcomeOK) {
		t.Fatalf("landing = %q, want %q", got, landing(outcomeOK))
	}

	rt, ctx := e.runtime("ledger")
	token, err := rt.OAuthAccessToken(ctx, "books")
	if err != nil || token != "a1" {
		t.Fatalf("OAuthAccessToken = %q, %v; want the access token the exchange kept", token, err)
	}
	company, err := rt.Secrets().Get(ctx, "company")
	if err != nil || string(company) != "company-42" {
		t.Fatalf("the unit's own secret = %q, %v; want what OnGrant kept", company, err)
	}
	if _, err := rt.Secrets().Get(ctx, extsecrets.CoreHeldKeyPrefix+grantKey("books")); !errors.Is(err, extsecrets.ErrReservedKey) {
		t.Fatalf("the unit read the core-held grant: %v", err)
	}

	if err := rt.OAuthDisconnect(ctx, "books"); err != nil {
		t.Fatalf("OAuthDisconnect: %v", err)
	}
	if _, err := rt.OAuthAccessToken(ctx, "books"); !errors.Is(err, extension.ErrOAuthNotConnected) {
		t.Fatalf("after disconnect = %v, want ErrOAuthNotConnected", err)
	}
}

func TestTheAppListNamesAUnitAppWithItsTitleUnitAndCallback(t *testing.T) {
	e := setupExtRuntime(t)
	setComposedExtensions([]extension.Extension{{Name: "ledger", Version: "0.1.0", OAuthApps: []extension.OAuthApp{booksApp("https://auth.ledgerly.test/token")}}})
	t.Cleanup(func() { setComposedExtensions(nil) })
	h := connectorAppHandlers{
		store:               capture.NewConnectorAppStore(NewSettingsStore(e.Pool), e.vault, nil).WithUnitApps(composedUnitApp),
		unitCallbackAPIBase: "https://api.test", unitCallbackPublicBase: "https://app.test",
	}
	if err := h.store.Set(e.callCtx(e.WS), "books", "ledgerly-client", "ledgerly-secret", ""); err != nil {
		t.Fatalf("storing the unit's app: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ListOauthApps(rec, httptest.NewRequest(http.MethodGet, "/v1/installation/oauth-apps", nil).WithContext(e.humanCtx(settingsReader)))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	var list crmcontracts.ConnectorAppList
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decoding the list: %v", err)
	}
	var providers []string
	for _, app := range list.Data {
		providers = append(providers, app.Provider)
	}
	if want := []string{"google", "microsoft", "books"}; !slices.Equal(providers, want) {
		t.Fatalf("listed providers = %v, want the core vendors then the unit's app %v", providers, want)
	}

	books := list.Data[2]
	if books.Title == nil || *books.Title != "Books ledger" || books.Unit == nil || *books.Unit != "ledger" {
		t.Fatalf("the unit app's title and unit = %v, %v; want the declaration's", books.Title, books.Unit)
	}
	if !books.Configured || books.ClientId != "ledgerly-client" || books.Source != crmcontracts.ConnectorAppSourceStored {
		t.Fatalf("the unit app = %+v, want the stored client id", books)
	}
	want := []crmcontracts.ConnectorAppRedirectUri{{
		Purpose: crmcontracts.ConnectorAppRedirectUriPurposeExtensionConnect,
		Url:     "https://api.test/v1/ext-oauth/books/callback",
	}}
	if !slices.Equal(books.RedirectUris, want) {
		t.Fatalf("the unit app's redirect URIs = %+v, want its consent callback %+v", books.RedirectUris, want)
	}
	if list.Data[0].Title != nil || list.Data[0].Unit != nil {
		t.Fatalf("a core vendor carries a unit's title or unit: %+v", list.Data[0])
	}
}

// settingsReader may read the installation's apps and nothing else.
var settingsReader = principal.Permissions{
	Objects: map[string]principal.ObjectGrant{capture.SettingsObject: {Read: true}},
}

// humanCtx is the workspace admin, signed in as a human and holding perms.
func (e *extRuntimeEnv) humanCtx(perms principal.Permissions) context.Context {
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.AdminUser.String(), UserID: e.AdminUser,
		Permissions: perms,
	})
}
