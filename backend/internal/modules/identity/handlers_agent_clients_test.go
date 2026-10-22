// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Every route answers a request with no signed-in human before it reads
// anything: an AI tool's clients are managed by a human, never an agent.
func TestEveryAgentClientRouteRefusesARequestWithNoHuman(t *testing.T) {
	h := Handlers{}
	id := openapi_types.UUID(ids.NewV7())
	for name, call := range map[string]func(http.ResponseWriter, *http.Request){
		"list": func(w http.ResponseWriter, r *http.Request) {
			h.ListAgentClients(w, r, crmcontracts.ListAgentClientsParams{})
		},
		"register": h.RegisterAgentClient,
		"update":   func(w http.ResponseWriter, r *http.Request) { h.UpdateAgentClient(w, r, id) },
		"delete":   func(w http.ResponseWriter, r *http.Request) { h.DeleteAgentClient(w, r, id) },
		"rotate":   func(w http.ResponseWriter, r *http.Request) { h.RotateAgentClientSecret(w, r, id) },
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			call(rec, httptest.NewRequest(http.MethodPost, "/installation/agent-clients", strings.NewReader(`{}`)))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s with no human → %d, want 401", name, rec.Code)
			}
		})
	}
}

// A body without disabled must not read as false: that would enable a client
// an admin disabled. Neither may it carry a field the operation does not take.
func TestAnUpdateWithoutDisabledIsRefusedBeforeTheStore(t *testing.T) {
	h := Handlers{}
	id := openapi_types.UUID(ids.NewV7())
	for name, body := range map[string]string{"empty": `{}`, "unknown field": `{"disabled": true, "name": "x"}`} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/installation/agent-clients/"+id.String(), strings.NewReader(body))
			h.UpdateAgentClient(rec, req.WithContext(withIdentity(req.Context(), Identity{})), id)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s → %d %s, want 422", body, rec.Code, rec.Body)
			}
		})
	}
}

func TestAFieldRefusalNamesTheField(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAgentClientError(rec, httptest.NewRequest(http.MethodPost, "/installation/agent-clients", nil),
		&InvalidAgentClientFieldError{Field: "redirect_uris", Code: "invalid_redirect_uri", Message: "must be https"})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "redirect_uris") {
		t.Errorf("got %d %s, want 422 naming redirect_uris", rec.Code, rec.Body)
	}
}

func TestTheWireClientCarriesNoSecretField(t *testing.T) {
	wire := wireAgentClient(AgentClient{ClientID: "c1", RegisteredBy: createdViaAdmin, Confidential: true, ConnectedUsers: 2})
	if wire.ClientId != "c1" || wire.RegisteredBy != crmcontracts.AgentClientRegisteredByAdmin || !wire.Confidential || wire.ConnectedUsers != 2 {
		t.Errorf("wire = %+v", wire)
	}
}

func TestRegisterInputIsTrimmedDeduplicatedAndBounded(t *testing.T) {
	name, uris, err := validAgentClientInput(RegisterAgentClientInput{
		Name: "  Gemini  ", RedirectURIs: []string{"https://a.example/cb", " https://a.example/cb ", "http://localhost/cb"},
	})
	if err != nil || name != "Gemini" || len(uris) != 2 {
		t.Fatalf("got %q %v %v, want the trimmed name and two distinct URIs", name, uris, err)
	}
	for label, in := range map[string]RegisterAgentClientInput{
		"blank name":    {Name: " ", RedirectURIs: []string{"https://a.example/cb"}},
		"long name":     {Name: strings.Repeat("n", maxAgentClientNameRunes+1), RedirectURIs: []string{"https://a.example/cb"}},
		"plain http":    {Name: "x", RedirectURIs: []string{"http://a.example/cb"}},
		"no uri":        {Name: "x"},
		"too many uris": {Name: "x", RedirectURIs: manyURIs(maxAgentClientRedirects + 1)},
	} {
		if _, _, err := validAgentClientInput(in); err == nil {
			t.Errorf("%s accepted", label)
		}
	}
}

func manyURIs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "https://a.example/cb" + strings.Repeat("x", i)
	}
	return out
}
