// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// Clients an admin registers by hand, the secret they may carry, and the
// switches over the two automatic methods, end to end over HTTP. The
// self-registered client every existing connection uses is tested beside them.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type registeredClient struct {
	Client struct {
		ID           string `json:"id"`
		ClientID     string `json:"client_id"`
		Confidential bool   `json:"confidential"`
		Disabled     bool   `json:"disabled"`
	} `json:"client"`
	ClientSecret string `json:"client_secret"`
}

// registerAdminClient registers a client through the admin route and points
// the env's flows at it, so authorize and exchange run as that client.
func (o *oauthEnv) registerAdminClient(t *testing.T, confidential bool) registeredClient {
	t.Helper()
	var out registeredClient
	if status := o.Call(t, "POST", "/v1/installation/agent-clients", integration.AnyMap{
		"name": "Gemini Enterprise", "redirect_uris": []string{oauthRedirect}, "confidential": confidential,
	}, nil, &out); status != http.StatusCreated {
		t.Fatalf("register → %d %+v", status, out)
	}
	if confidential == (out.ClientSecret == "") {
		t.Fatalf("confidential=%v but client_secret=%q", confidential, out.ClientSecret)
	}
	o.clientID = out.Client.ClientID
	return out
}

func basicAuth(id, secret string) map[string]string {
	raw := url.QueryEscape(id) + ":" + url.QueryEscape(secret)
	return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))}
}

// postTokenWith is postToken with request headers, for client authentication
// in an HTTP Basic header.
func (o *oauthEnv) postTokenWith(t *testing.T, path string, form url.Values, headers map[string]string) (int, map[string]any, http.Header) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, o.TS.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := o.Client.Do(req) //nolint:bodyclose // closed by apptest.CloseBody below; bodyclose only recognises a Close in the same package
	if err != nil {
		t.Fatal(err)
	}
	defer apptest.CloseBody(t, resp)
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("%s response is not JSON: %v", path, err)
	}
	return resp.StatusCode, body, resp.Header
}

func (o *oauthEnv) codeForm(code string) url.Values {
	return url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {o.clientID},
		"redirect_uri": {oauthRedirect}, "code_verifier": {o.verifier},
	}
}

func TestAClientWithASecretConnectsAndRenewsOnlyWithIt(t *testing.T) {
	o := setupOAuth(t)
	client := o.registerAdminClient(t, true)
	code := o.authorize(t, url.Values{"scope": {"read write offline_access"}})

	// The failed attempts come first: client authentication runs before the
	// code is read, so they must not spend the code.
	if status, body, header := o.postTokenWith(t, "/oauth/token", o.codeForm(code), nil); status != http.StatusUnauthorized || body["error"] != "invalid_client" || header.Get("WWW-Authenticate") != "" {
		t.Fatalf("no secret → %d %v, want 401 invalid_client with no Basic challenge", status, body)
	}
	if status, body, header := o.postTokenWith(t, "/oauth/token", o.codeForm(code), basicAuth(o.clientID, "wrong")); status != http.StatusUnauthorized || header.Get("WWW-Authenticate") == "" {
		t.Fatalf("wrong Basic secret → %d %v, want 401 asking for Basic", status, body)
	}
	both := o.codeForm(code)
	both.Set("client_secret", client.ClientSecret)
	if status, body, _ := o.postTokenWith(t, "/oauth/token", both, basicAuth(o.clientID, client.ClientSecret)); status != http.StatusBadRequest || body["error"] != "invalid_request" {
		t.Fatalf("secret in both places → %d %v, want 400 invalid_request", status, body)
	}

	status, body, _ := o.postTokenWith(t, "/oauth/token", o.codeForm(code), basicAuth(o.clientID, client.ClientSecret))
	if status != http.StatusOK {
		t.Fatalf("Basic secret → %d %v", status, body)
	}
	refresh := body["refresh_token"].(string)
	if listTools(t, o.AppEnv, body["access_token"].(string)).StatusCode != http.StatusOK {
		t.Fatal("the access token a confidential client was issued does not reach /mcp")
	}

	renew := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {o.clientID}}
	// A refresh without the secret is refused before reuse detection, so it
	// cannot end the connection it cannot renew.
	if status, body, _ := o.postTokenWith(t, "/oauth/token", renew, nil); status != http.StatusUnauthorized {
		t.Fatalf("refresh without the secret → %d %v, want 401", status, body)
	}
	renew.Set("client_secret", client.ClientSecret)
	if status, body, _ := o.postTokenWith(t, "/oauth/token", renew, nil); status != http.StatusOK {
		t.Fatalf("refresh with the secret in the form, after a refused attempt → %d %v", status, body)
	}

	// The token is spent now, so presenting it again is reuse, which ends the
	// connection. Without the secret the caller is not the client, and a
	// stranger must not be able to end it.
	replay := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {o.clientID}}
	if status, body, _ := o.postTokenWith(t, "/oauth/token", replay, basicAuth(o.clientID, "wrong")); status != http.StatusUnauthorized || body["error"] != "invalid_client" {
		t.Fatalf("replaying the spent token with a wrong secret → %d %v, want 401 invalid_client", status, body)
	}
	if o.grantRevoked(t) {
		t.Fatal("a replay by a caller without the secret revoked the connection")
	}
}

// Every self-registered connection is a public client with no secret. A
// library that also sends a Basic header with an empty secret, or an empty
// client_secret, must keep working.
func TestASelfRegisteredClientStillConnectsWithNoSecret(t *testing.T) {
	o := setupOAuth(t)
	for name, headers := range map[string]map[string]string{
		"form only":             nil,
		"basic with no secret":  basicAuth(o.clientID, ""),
		"basic and form secret": basicAuth(o.clientID, ""),
	} {
		t.Run(name, func(t *testing.T) {
			code := o.authorize(t, url.Values{"scope": {"read offline_access"}})
			form := o.codeForm(code)
			if name == "basic and form secret" {
				form.Set("client_secret", "")
			}
			status, body, _ := o.postTokenWith(t, "/oauth/token", form, headers)
			if status != http.StatusOK {
				t.Fatalf("exchange → %d %v", status, body)
			}
			renew := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {body["refresh_token"].(string)}, "client_id": {o.clientID}}
			if status, body, _ := o.postTokenWith(t, "/oauth/token", renew, headers); status != http.StatusOK {
				t.Fatalf("refresh → %d %v", status, body)
			}
		})
	}
}

func TestRotatingASecretCutsTheOldOneAndKeepsTheConnection(t *testing.T) {
	o := setupOAuth(t)
	client := o.registerAdminClient(t, true)
	code := o.authorize(t, url.Values{"scope": {"read offline_access"}})
	_, body, _ := o.postTokenWith(t, "/oauth/token", o.codeForm(code), basicAuth(o.clientID, client.ClientSecret))
	refresh := body["refresh_token"].(string)

	var rotated struct {
		ClientSecret string `json:"client_secret"`
	}
	if status := o.Call(t, "POST", "/v1/installation/agent-clients/"+client.Client.ID+"/secret", nil, nil, &rotated); status != http.StatusOK || rotated.ClientSecret == "" || rotated.ClientSecret == client.ClientSecret {
		t.Fatalf("rotate → %d %+v", status, rotated)
	}
	renew := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}}
	if status, _, _ := o.postTokenWith(t, "/oauth/token", renew, basicAuth(o.clientID, client.ClientSecret)); status != http.StatusUnauthorized {
		t.Fatalf("the old secret after rotation → %d, want 401", status)
	}
	if status, body, _ := o.postTokenWith(t, "/oauth/token", renew, basicAuth(o.clientID, rotated.ClientSecret)); status != http.StatusOK {
		t.Fatalf("the new secret renews the same connection → %d %v", status, body)
	}

	public := o.registerAdminClient(t, false)
	var problem struct {
		Code string `json:"code"`
	}
	if status := o.Call(t, "POST", "/v1/installation/agent-clients/"+public.Client.ID+"/secret", nil, nil, &problem); status != http.StatusConflict {
		t.Fatalf("rotating a public client → %d %+v, want 409", status, problem)
	}
}

func TestDisableBlocksLiveTokensAndEnableBringsThemBack(t *testing.T) {
	o := setupOAuth(t)
	client := o.registerAdminClient(t, false)
	code := o.authorize(t, nil)
	_, body, _ := o.postTokenWith(t, "/oauth/token", o.codeForm(code), nil)
	token := body["access_token"].(string)

	path := "/v1/installation/agent-clients/" + client.Client.ID
	if status := o.Call(t, "PATCH", path, integration.AnyMap{"disabled": true}, nil, nil); status != http.StatusOK {
		t.Fatalf("disable → %d", status)
	}
	if got := listTools(t, o.AppEnv, token).StatusCode; got != http.StatusUnauthorized {
		t.Fatalf("a disabled client's live token → %d, want 401", got)
	}
	if status := o.Call(t, "PATCH", path, integration.AnyMap{"disabled": false}, nil, nil); status != http.StatusOK {
		t.Fatalf("enable → %d", status)
	}
	if got := listTools(t, o.AppEnv, token).StatusCode; got != http.StatusOK {
		t.Fatalf("the token after enable → %d, want 200", got)
	}
	assertOwnerCount(t, o, 2, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'oauth_client' AND action IN ('pause', 'resume') AND entity_id = $1`, client.Client.ID)
}

func TestDeletingAClientEndsEveryConnectionUnderIt(t *testing.T) {
	o := setupOAuth(t)
	client := o.registerAdminClient(t, false)
	code := o.authorize(t, url.Values{"scope": {"read offline_access"}})
	_, body, _ := o.postTokenWith(t, "/oauth/token", o.codeForm(code), nil)

	if status := o.Call(t, "DELETE", "/v1/installation/agent-clients/"+client.Client.ID, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete → %d", status)
	}
	if got := listTools(t, o.AppEnv, body["access_token"].(string)).StatusCode; got != http.StatusUnauthorized {
		t.Fatalf("a deleted client's token → %d, want 401", got)
	}
	assertOwnerCount(t, o, 0, `SELECT count(*) FROM oauth_grant WHERE client_id = $1 AND revoked_at IS NULL`, o.clientID)
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	o.Call(t, "GET", "/v1/installation/agent-clients", nil, nil, &list)
	for _, c := range list.Data {
		if c.ID == client.Client.ID {
			t.Fatal("a deleted client is still listed")
		}
	}
	if status := o.Call(t, "DELETE", "/v1/installation/agent-clients/"+client.Client.ID, nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("deleting it again → %d, want 404", status)
	}
}

// A switch stops new sign-ins through its method, a returning human included,
// and never a connection that is already renewing.
func TestSwitchesStopNewSignInsAndNeverARenewal(t *testing.T) {
	o := setupOAuth(t)
	code := o.authorize(t, url.Values{"scope": {"read offline_access"}})
	_, body, _ := o.postTokenWith(t, "/oauth/token", o.codeForm(code), nil)
	// A human looking at the consent screen while the admin switches the
	// method off.
	approval := o.armConsent(t, nil)
	approval.Set("scopes", "read")

	if status := o.Call(t, "PATCH", "/v1/installation/settings", integration.AnyMap{
		"dynamic_client_registration": false, "metadata_document_clients": false,
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("switching both methods off → %d", status)
	}

	var metadata map[string]any
	o.Call(t, "GET", "/.well-known/oauth-authorization-server", nil, nil, &metadata)
	if _, ok := metadata["registration_endpoint"]; ok {
		t.Errorf("discovery still advertises registration_endpoint: %v", metadata)
	}
	if _, ok := metadata["client_id_metadata_document_supported"]; ok {
		t.Errorf("discovery still advertises CIMD: %v", metadata)
	}
	if status := o.Call(t, "POST", "/oauth/register", integration.AnyMap{
		"client_name": "late", "redirect_uris": []string{oauthRedirect},
	}, nil, nil); status != http.StatusNotFound {
		t.Errorf("register with self-registration off → %d, want 404", status)
	}
	status, location, _, _ := o.authorizeNoFollow(t, nil)
	if status != http.StatusFound || consentFragment(t, location).Get("error") != "method_off" {
		t.Errorf("sign-in through a self-registered client → %d %q, want the consent screen's method_off", status, location)
	}
	if status, location, _ := o.postConsent(t, approval); status != http.StatusFound || consentFragment(t, location).Get("error") != "method_off" {
		t.Errorf("approving on a screen opened before the switch → %d %q, want the consent screen's method_off", status, location)
	}
	// The redirect is judged before the method, so a URI the client never
	// registered is answered to the caller and never put on the screen.
	if status, body := o.authorizeRaw(t, url.Values{"redirect_uri": {"https://elsewhere.example/cb"}}); status != http.StatusBadRequest || !strings.Contains(body, "invalid_request") {
		t.Errorf("sign-in to an unregistered redirect with the method off → %d %s, want 400 invalid_request", status, body)
	}

	renew := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {body["refresh_token"].(string)}, "client_id": {o.clientID}}
	if status, body, _ := o.postTokenWith(t, "/oauth/token", renew, nil); status != http.StatusOK {
		t.Fatalf("an existing connection renews with the switches off → %d %v", status, body)
	}

	// A client an admin registers is not one of the switched methods.
	o.registerAdminClient(t, false)
	o.authorize(t, nil)
}

func TestTheSecretIsInNoAuditRowAndNoListResponse(t *testing.T) {
	o := setupOAuth(t)
	client := o.registerAdminClient(t, true)
	var rotated struct {
		ClientSecret string `json:"client_secret"`
	}
	o.Call(t, "POST", "/v1/installation/agent-clients/"+client.Client.ID+"/secret", nil, nil, &rotated)

	for _, secret := range []string{client.ClientSecret, rotated.ClientSecret} {
		for _, needle := range []string{secret, sha256Hex(secret)} {
			assertOwnerCount(t, o, 0, `SELECT count(*) FROM audit_log
				WHERE strpos(coalesce(before::text, '') || coalesce(after::text, '') || coalesce(evidence::text, ''), $1) > 0`, needle)
		}
	}
	var raw json.RawMessage
	o.Call(t, "GET", "/v1/installation/agent-clients", nil, nil, &raw)
	if strings.Contains(string(raw), rotated.ClientSecret) || strings.Contains(string(raw), "secret_hash") {
		t.Fatalf("the list response carries secret material: %s", raw)
	}
	assertOwnerCount(t, o, 1, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'oauth_client' AND action = 'create' AND entity_id = $1`, client.Client.ID)
}

// RFC 7009: a confidential client's token is handed back only by that client.
func TestOnlyTheConfidentialClientItselfRevokesItsToken(t *testing.T) {
	o := setupOAuth(t)
	client := o.registerAdminClient(t, true)
	code := o.authorize(t, nil)
	_, body, _ := o.postTokenWith(t, "/oauth/token", o.codeForm(code), basicAuth(o.clientID, client.ClientSecret))
	token := body["access_token"].(string)

	if status, _, _ := o.postTokenWith(t, "/oauth/revoke", url.Values{"token": {token}}, nil); status != http.StatusOK {
		t.Fatalf("revoke without credentials → %d, want the non-disclosing 200", status)
	}
	if listTools(t, o.AppEnv, token).StatusCode != http.StatusOK {
		t.Fatal("a revoke without the client's secret ended its connection")
	}
	if status, _, _ := o.postTokenWith(t, "/oauth/revoke", url.Values{"token": {token}}, basicAuth(o.clientID, "wrong")); status != http.StatusUnauthorized {
		t.Fatalf("revoke with a wrong secret → %d, want 401", status)
	}
	if status, _, _ := o.postTokenWith(t, "/oauth/revoke", url.Values{"token": {token}}, basicAuth(o.clientID, client.ClientSecret)); status != http.StatusOK {
		t.Fatalf("revoke with the secret → %d", status)
	}
	if listTools(t, o.AppEnv, token).StatusCode != http.StatusUnauthorized {
		t.Fatal("the client's own authenticated revoke left its token live")
	}
}

func TestTheSignInReplyNamesTheIssuerDiscoveryAdvertises(t *testing.T) {
	o := setupOAuth(t)
	form := o.armConsent(t, nil)
	form.Set("scopes", "read")
	_, location, _ := o.postConsent(t, form)
	granted, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Issuer string `json:"issuer"`
		ISS    bool   `json:"authorization_response_iss_parameter_supported"`
	}
	o.Call(t, "GET", "/.well-known/oauth-authorization-server", nil, nil, &metadata)
	if !metadata.ISS || granted.Query().Get("iss") != metadata.Issuer || metadata.Issuer == "" {
		t.Fatalf("iss = %q, issuer = %q, advertised = %v", granted.Query().Get("iss"), metadata.Issuer, metadata.ISS)
	}
}

func TestTheClientListRefusesRedirectsItCannotServe(t *testing.T) {
	o := setupOAuth(t)
	for name, uris := range map[string][]string{
		"plain http":  {"http://evil.example/cb"},
		"a fragment":  {"https://client.example/cb#x"},
		"none at all": {},
	} {
		t.Run(name, func(t *testing.T) {
			if status := o.Call(t, "POST", "/v1/installation/agent-clients", integration.AnyMap{
				"name": "x", "redirect_uris": uris, "confidential": false,
			}, nil, nil); status != http.StatusUnprocessableEntity {
				t.Errorf("register with %v → %d, want 422", uris, status)
			}
		})
	}
}
