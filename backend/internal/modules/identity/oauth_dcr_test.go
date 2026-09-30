// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// thirdPartyRegistration is the document a general-purpose MCP client sends:
// the members this server reads plus several it has no use for.
const thirdPartyRegistration = `{
	"client_name": "Le Chat",
	"redirect_uris": ["https://chat.example/connections/oauth/callback"],
	"token_endpoint_auth_method": "none",
	"grant_types": ["authorization_code", "refresh_token"],
	"response_types": ["code"],
	"scope": "read write",
	"client_uri": "https://chat.example",
	"logo_uri": "https://chat.example/logo.png",
	"software_id": "chat-connector",
	"software_version": "2.0.0"
}`

func parseDocument(t *testing.T, document string) (dcrRequest, error) {
	t.Helper()
	var members map[string]json.RawMessage
	if err := json.Unmarshal([]byte(document), &members); err != nil {
		t.Fatalf("test document is not JSON: %v", err)
	}
	return parseDCR(members)
}

func TestRegistrationIgnoresMetadataItDoesNotUnderstand(t *testing.T) {
	req, err := parseDocument(t, thirdPartyRegistration)
	if err != nil {
		t.Fatalf("parseDCR refused a conforming document: %v", err)
	}
	if req.ClientName != "Le Chat" || len(req.RedirectURIs) != 1 {
		t.Errorf("parsed %+v, want the client name and its one redirect", req)
	}
}

func TestRegistrationEchoesDefaultsForOmittedGrantAndResponseTypes(t *testing.T) {
	req, err := parseDocument(t, `{"client_name":"bare","redirect_uris":["https://client.example/cb"]}`)
	if err != nil {
		t.Fatal(err)
	}
	echo := req.registered("client-1")
	if !slices.Equal(echo.GrantTypes, []string{oauthGrantAuthorizationCode}) {
		t.Errorf("grant_types = %v, want the RFC 7591 default", echo.GrantTypes)
	}
	if !slices.Equal(echo.ResponseTypes, []string{oauthResponseTypeCode}) {
		t.Errorf("response_types = %v, want the RFC 7591 default", echo.ResponseTypes)
	}
}

func TestRegistrationRefusesWhatItCannotHonour(t *testing.T) {
	for name, tc := range map[string]struct {
		document, code string
	}{
		"implicit grant": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"grant_types":["implicit"]}`,
			"invalid_client_metadata"},
		"token response type": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"response_types":["token"]}`,
			"invalid_client_metadata"},
		"confidential client": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"token_endpoint_auth_method":"client_secret_basic"}`,
			"invalid_client_metadata"},
		"plain-http redirect": {
			`{"client_name":"x","redirect_uris":["http://c.example/cb"]}`,
			"invalid_redirect_uri"},
		"no redirect": {`{"client_name":"x"}`, "invalid_client_metadata"},
		"mistyped member": {
			`{"client_name":"x","redirect_uris":"https://c.example/cb"}`,
			"invalid_client_metadata"},
		// Member names are case-sensitive: a near-spelling is an unknown member,
		// ignored, so the required one is still missing.
		"case-folded name": {
			`{"Client_Name":"x","redirect_uris":["https://c.example/cb"]}`,
			"invalid_client_metadata"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseDocument(t, tc.document)
			var refusal *dcrMetadataError
			if !errors.As(err, &refusal) {
				t.Fatalf("parseDCR = %v, want a registration refusal", err)
			}
			if refusal.code != tc.code {
				t.Errorf("error = %q (%s), want %q", refusal.code, refusal.description, tc.code)
			}
		})
	}
}

// Past the metadata checks, a conforming document reaches the workspace
// binding — which a workspace-less handler refuses as invalid_request, not
// as a malformed document.
func TestRegisterEndpointAcceptsAThirdPartyDocument(t *testing.T) {
	rec := httptest.NewRecorder()
	workspacelessHandlers().oauthRegister(rec,
		httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(thirdPartyRegistration)))

	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("register body is not an oauth document: %v", err)
	}
	if body.Error != "invalid_request" {
		t.Errorf("register error = %q, want the workspace refusal %q", body.Error, "invalid_request")
	}
}

func TestRegisterEndpointRefusesANonObjectDocument(t *testing.T) {
	rec := httptest.NewRecorder()
	workspacelessHandlers().oauthRegister(rec,
		httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`["client_name"]`)))

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_client_metadata") {
		t.Errorf("register → %d %s, want 400 invalid_client_metadata", rec.Code, rec.Body.String())
	}
}
