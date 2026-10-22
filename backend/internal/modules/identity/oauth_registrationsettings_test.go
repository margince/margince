// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func switchesReturning(m RegistrationMethods, err error) AdminRegistrationSwitches {
	return func(context.Context) (RegistrationMethods, error) { return m, err }
}

var allMethods = RegistrationMethods{Dynamic: true, MetadataDocument: true}

func TestAnUnwiredServerKeepsBothMethodsOn(t *testing.T) {
	got, err := Handlers{}.registrationMethods(context.Background())
	if err != nil || got != allMethods {
		t.Errorf("got %+v, %v; a server built without the switches must behave as before them", got, err)
	}
}

// The deployment file is a ceiling: an admin's switch can narrow it, never
// widen it.
func TestTheEffectiveMethodIsTheDeploymentAndTheSwitch(t *testing.T) {
	h := Handlers{}.WithClientRegistration(
		RegistrationMethods{Dynamic: false, MetadataDocument: true},
		switchesReturning(RegistrationMethods{Dynamic: true, MetadataDocument: false}, nil))
	got, err := h.registrationMethods(context.Background())
	if err != nil || got.Dynamic || got.MetadataDocument {
		t.Errorf("got %+v, %v; want both off: one by the file, one by the admin", got, err)
	}
}

func TestEachMethodAdmitsOnlyItsOwnClients(t *testing.T) {
	onlyDCR := RegistrationMethods{Dynamic: true}
	for via, want := range map[string]bool{"dcr": true, "cimd": false, "admin": true} {
		if got := onlyDCR.admits(via); got != want {
			t.Errorf("admits(%q) = %v, want %v", via, got, want)
		}
	}
	if !(RegistrationMethods{}).admits("admin") {
		t.Error("a client an admin registered must be admitted with both methods off")
	}
}

func discoveryWith(t *testing.T, h Handlers) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.OAuthServerMetadata(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestDiscoveryOffersOnlyTheMethodsThatAreOn(t *testing.T) {
	base := Handlers{mcpResource: "https://crm.example.com/mcp"}
	for name, tc := range map[string]struct {
		switches       AdminRegistrationSwitches
		register, cimd bool
	}{
		"both on":         {switchesReturning(allMethods, nil), true, true},
		"dcr off":         {switchesReturning(RegistrationMethods{MetadataDocument: true}, nil), false, true},
		"cimd off":        {switchesReturning(RegistrationMethods{Dynamic: true}, nil), true, false},
		"unreadable read": {switchesReturning(RegistrationMethods{}, errors.New("db down")), false, false},
	} {
		t.Run(name, func(t *testing.T) {
			doc := discoveryWith(t, base.WithClientRegistration(allMethods, tc.switches))
			_, register := doc["registration_endpoint"]
			_, cimd := doc["client_id_metadata_document_supported"]
			if register != tc.register || cimd != tc.cimd {
				t.Errorf("registration_endpoint=%v cimd=%v, want %v %v", register, cimd, tc.register, tc.cimd)
			}
			if doc["authorization_response_iss_parameter_supported"] != true {
				t.Error("discovery must say the sign-in reply carries iss")
			}
		})
	}
}

func TestRegistrationIsGoneWhenSelfRegistrationIsOff(t *testing.T) {
	h := Handlers{}.WithClientRegistration(allMethods, switchesReturning(RegistrationMethods{MetadataDocument: true}, nil))
	rec := httptest.NewRecorder()
	h.oauthRegister(rec, httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{}`)))
	if rec.Code != http.StatusNotFound {
		t.Errorf("register with DCR off → %d, want 404", rec.Code)
	}
}

func TestRegistrationIsUnavailableWhenTheSwitchCannotBeRead(t *testing.T) {
	h := Handlers{}.WithClientRegistration(allMethods, switchesReturning(RegistrationMethods{}, errors.New("db down")))
	rec := httptest.NewRecorder()
	h.oauthRegister(rec, httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{}`)))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), oauthErrTemporarilyUnavailable) {
		t.Errorf("register with an unreadable switch → %d %s, want 503 temporarily_unavailable", rec.Code, rec.Body)
	}
}
