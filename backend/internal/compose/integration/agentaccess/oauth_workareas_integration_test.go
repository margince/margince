// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// The areas a human chooses on the consent screen bound the connection, and a
// renewal keeps them. A change on the passport page holds for the live
// credential and the next one. The same refusal answers on REST.

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

const adminSettingsRoute = "/v1/roles"

func (o *oauthEnv) consentWithAreas(t *testing.T, areas string) map[string]any {
	t.Helper()
	form := o.armConsent(t, url.Values{"scope": {"read offline_access"}})
	form.Set("scopes", "read")
	form.Set("areas", areas)
	status, location, body := o.postConsent(t, form)
	if status != http.StatusFound {
		t.Fatalf("consent POST → %d %s", status, body)
	}
	granted, err := url.Parse(location)
	if err != nil || granted.Query().Get("code") == "" {
		t.Fatalf("the consent redirect carries no code: %q", location)
	}
	code, answer := o.exchange(t, url.Values{"code": {granted.Query().Get("code")}})
	if code != http.StatusOK {
		t.Fatalf("code exchange → %d %v", code, answer)
	}
	return answer
}

func (o *oauthEnv) renewed(t *testing.T, tokens map[string]any) map[string]any {
	t.Helper()
	refresh, _ := tokens["refresh_token"].(string)
	status, renewed := o.renew(t, refresh, nil)
	if status != http.StatusOK {
		t.Fatalf("refresh → %d %v", status, renewed)
	}
	return renewed
}

func (o *oauthEnv) readsAdminSettings(t *testing.T, tokens map[string]any) int {
	t.Helper()
	access, _ := tokens["access_token"].(string)
	return o.Call(t, http.MethodGet, adminSettingsRoute, nil, map[string]string{"Authorization": "Bearer " + access}, nil)
}

func TestTheAreasChosenAtConsentHoldAcrossARenewalUntilTheHumanChangesThem(t *testing.T) {
	o := setupOAuth(t)
	tokens := o.consentWithAreas(t, "business")
	if status := o.readsAdminSettings(t, tokens); status != http.StatusForbidden {
		t.Fatalf("a connection consented to sales work only reads admin settings → %d, want 403", status)
	}
	tokens = o.renewed(t, tokens)
	if status := o.readsAdminSettings(t, tokens); status != http.StatusForbidden {
		t.Fatalf("its renewed credential reads admin settings → %d, want 403: a renewal keeps the consent's areas", status)
	}

	var listed struct {
		Data []struct {
			ID    string `json:"id"`
			Areas struct {
				Mode  string   `json:"mode"`
				Areas []string `json:"areas"`
			} `json:"areas"`
		} `json:"data"`
	}
	if status := o.Call(t, http.MethodGet, "/v1/passports", nil, nil, &listed); status != http.StatusOK || len(listed.Data) != 1 {
		t.Fatalf("list passports → %d %+v, want the one connection", status, listed.Data)
	}
	if got := listed.Data[0].Areas; got.Mode != "custom" || len(got.Areas) != 1 || got.Areas[0] != "business" {
		t.Fatalf("the connection lists areas %+v, want sales work alone", got)
	}
	if status := o.Call(t, http.MethodPut, "/v1/passports/"+listed.Data[0].ID+"/areas",
		integration.AnyMap{"mode": "all", "areas": []string{}}, nil, nil); status != http.StatusOK {
		t.Fatalf("setting the connection to all areas → %d", status)
	}
	if status := o.readsAdminSettings(t, tokens); status != http.StatusOK {
		t.Errorf("the live credential reads admin settings → %d once the human set all areas, want 200", status)
	}
	if status := o.readsAdminSettings(t, o.renewed(t, tokens)); status != http.StatusOK {
		t.Errorf("the next renewal reads admin settings → %d, want 200: the grant keeps the change", status)
	}
}

// A choice the screen never renders is refused like a scope outside the
// vocabulary: back to the screen, with no code minted.
func TestAConsentNamingAnAreaNoScreenOffersMintsNoCode(t *testing.T) {
	o := setupOAuth(t)
	form := o.armConsent(t, url.Values{"scope": {"read"}})
	form.Set("scopes", "read")
	form.Set("areas", "business operator")
	status, location, _ := o.postConsent(t, form)
	if granted, err := url.Parse(location); status != http.StatusFound || err != nil || granted.Query().Get("code") != "" {
		t.Fatalf("a consent naming the operator area → %d %q, want a refusal with no code", status, location)
	}
	assertOwnerCount(t, o, 0, `SELECT count(*) FROM oauth_authorization_code`)
}

// A rep's screen never offers admin settings, so a consent naming it is refused
// as a hand-built form is.
func TestAConsentNamingAnAreaTheRoleDoesNotReachMintsNoCode(t *testing.T) {
	o := setupOAuth(t)
	var email string
	if err := o.Owner.QueryRow(t.Context(), `SELECT email FROM app_user WHERE id = $1`, apptest.SessionUserID(t, o.AppEnv)).Scan(&email); err != nil {
		t.Fatalf("reading the session's email: %v", err)
	}
	apptest.AssignRoleTo(t, o.AppEnv, email, "rep")
	form := o.armConsent(t, url.Values{"scope": {"read"}})
	form.Set("scopes", "read")
	form.Set("areas", "admin_settings")
	status, location, _ := o.postConsent(t, form)
	if granted, err := url.Parse(location); status != http.StatusFound || err != nil || granted.Query().Get("code") != "" {
		t.Fatalf("a rep consenting to admin settings → %d %q, want a refusal with no code", status, location)
	}
	assertOwnerCount(t, o, 0, `SELECT count(*) FROM oauth_authorization_code`)
}
