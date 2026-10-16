// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit's OAuth app over real Postgres and the real stores. The client
// credentials sit in the installation app store and the grant in the unit's
// namespace, where the unit's own Secrets port cannot reach it.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

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
	admin := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.AdminUser.String(), UserID: e.AdminUser,
		Permissions: adminRBAC().Permissions,
	})
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
