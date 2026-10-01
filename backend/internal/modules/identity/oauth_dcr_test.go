// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"encoding/json"
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

func parseDocument(t *testing.T, document string) (dcrRequest, *dcrRefusal) {
	t.Helper()
	var members map[string]json.RawMessage
	if err := json.Unmarshal([]byte(document), &members); err != nil {
		t.Fatalf("test document is not JSON: %v", err)
	}
	return parseDCR(members)
}

func TestRegistrationIgnoresMetadataItDoesNotUnderstand(t *testing.T) {
	req, refusal := parseDocument(t, thirdPartyRegistration)
	if refusal != nil {
		t.Fatalf("parseDCR refused a conforming document: %s", refusal.description)
	}
	if req.ClientName != "Le Chat" || len(req.RedirectURIs) != 1 {
		t.Errorf("parsed %+v, want the client name and its one redirect", req)
	}
}

// The echo is what the server does, not what was asked: a client that
// registered authorization_code alone still receives refresh tokens when it
// asks for offline_access, so telling it otherwise would be false.
func TestRegistrationEchoesTheGrantsTheServerIssues(t *testing.T) {
	req, refusal := parseDocument(t,
		`{"client_name":"narrow","redirect_uris":["https://client.example/cb"],"grant_types":["authorization_code"]}`)
	if refusal != nil {
		t.Fatal(refusal.description)
	}
	echo := req.registered("client-1")
	if !slices.Equal(echo.GrantTypes, oauthGrantTypesSupported) {
		t.Errorf("grant_types = %v, want what the server issues %v", echo.GrantTypes, oauthGrantTypesSupported)
	}
	if !slices.Equal(echo.ResponseTypes, []string{oauthResponseTypeCode}) {
		t.Errorf("response_types = %v, want [code]", echo.ResponseTypes)
	}
}

func TestRegistrationReadsAnEmptyListAsOmitted(t *testing.T) {
	req, refusal := parseDocument(t,
		`{"client_name":"x","redirect_uris":["https://c.example/cb"],"grant_types":[],"response_types":[]}`)
	if refusal != nil {
		t.Fatalf("parseDCR refused empty lists: %s", refusal.description)
	}
	if echo := req.registered("client-1"); !slices.Equal(echo.GrantTypes, oauthGrantTypesSupported) {
		t.Errorf("grant_types = %v, want what the server issues", echo.GrantTypes)
	}
}

func TestRegistrationRefusesWhatItCannotHonour(t *testing.T) {
	for name, tc := range map[string]struct {
		document, code string
	}{
		"implicit grant": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"grant_types":["implicit"]}`,
			"invalid_client_metadata",
		},
		"refresh without a code": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"grant_types":["refresh_token"]}`,
			"invalid_client_metadata",
		},
		"token response type": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"response_types":["token"]}`,
			"invalid_client_metadata",
		},
		"confidential client": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"token_endpoint_auth_method":"client_secret_basic"}`,
			"invalid_client_metadata",
		},
		"plain-http redirect": {
			`{"client_name":"x","redirect_uris":["http://c.example/cb"]}`,
			"invalid_redirect_uri",
		},
		"no redirect": {`{"client_name":"x"}`, "invalid_client_metadata"},
		"grant beside the code grant": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"grant_types":["authorization_code","implicit"]}`,
			"invalid_client_metadata",
		},
		"mistyped redirect_uris": {
			`{"client_name":"x","redirect_uris":"https://c.example/cb"}`,
			"invalid_client_metadata",
		},
		"mistyped client_name": {
			`{"client_name":7,"redirect_uris":["https://c.example/cb"]}`,
			"invalid_client_metadata",
		},
		"mistyped token_endpoint_auth_method": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"token_endpoint_auth_method":["none"]}`,
			"invalid_client_metadata",
		},
		"mistyped grant_types": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"grant_types":"authorization_code"}`,
			"invalid_client_metadata",
		},
		"mistyped response_types": {
			`{"client_name":"x","redirect_uris":["https://c.example/cb"],"response_types":"code"}`,
			"invalid_client_metadata",
		},
		// Member names are case-sensitive: a near-spelling is an unknown member,
		// ignored, so the required one is still missing.
		"case-folded name": {
			`{"Client_Name":"x","redirect_uris":["https://c.example/cb"]}`,
			"invalid_client_metadata",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, refusal := parseDocument(t, tc.document)
			if refusal == nil {
				t.Fatal("parseDCR accepted the document, want a registration refusal")
			}
			if refusal.code != tc.code {
				t.Errorf("error = %q (%s), want %q", refusal.code, refusal.description, tc.code)
			}
		})
	}
}

func TestRegisterEndpointParsesAThirdPartyDocumentBeforeResolvingTheWorkspace(t *testing.T) {
	rec := httptest.NewRecorder()
	workspacelessHandlers().oauthRegister(rec,
		httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(thirdPartyRegistration)))

	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("register body is not an oauth document: %v", err)
	}
	if rec.Code != http.StatusBadRequest || body.Error != "invalid_request" {
		t.Errorf("register → %d %q, want the workspace refusal 400 %q", rec.Code, body.Error, "invalid_request")
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

func TestRegisterEndpointNamesTheMemberItRefuses(t *testing.T) {
	rec := httptest.NewRecorder()
	workspacelessHandlers().oauthRegister(rec, httptest.NewRequest(http.MethodPost, "/oauth/register",
		strings.NewReader(`{"client_name":"x","redirect_uris":["https://c.example/cb"],"grant_types":["implicit"]}`)))

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "grant_types") {
		t.Errorf("register → %d %s, want 400 naming grant_types", rec.Code, rec.Body.String())
	}
}

func TestRegisterEndpointRefusesAnOversizedDocument(t *testing.T) {
	oversized := `{"client_name":"` + strings.Repeat("x", 2<<20) + `"}`
	rec := httptest.NewRecorder()
	workspacelessHandlers().oauthRegister(rec,
		httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(oversized)))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("register → %d, want 413", rec.Code)
	}
}
