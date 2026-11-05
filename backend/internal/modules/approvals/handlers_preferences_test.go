// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPersonalPreferenceRequiresAnExplicitKindAndChoice(t *testing.T) {
	for _, test := range []struct{ body, field string }{
		{`{}`, "muted"},
		{`{"kind":"close_date_correction"}`, "muted"},
		{`{"kind":"close_date_correction","muted":null}`, "muted"},
		{`{"muted":false}`, "kind"},
		{`{"kind":null,"muted":true}`, "kind"},
	} {
		t.Run(test.body, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPatch, "/me/approval-preferences", strings.NewReader(test.body))
			request = request.WithContext(principal.WithActor(request.Context(), principal.Principal{Type: principal.PrincipalHuman, ID: "user:reviewer"}))
			recorder := httptest.NewRecorder()
			Handlers{}.UpdateApprovalPreference(recorder, request)
			if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), test.field) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPersonalPreferenceHTTPRefusesMalformedBodiesAndAgentReaders(t *testing.T) {
	for _, test := range []struct {
		method, body string
		status       int
	}{
		{http.MethodPatch, `{"muted":`, http.StatusUnprocessableEntity},
		{http.MethodPatch, `{"kind":"close_date_correction","muted":false}`, http.StatusForbidden},
		{http.MethodGet, "", http.StatusForbidden},
	} {
		t.Run(test.method+test.body, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/me/approval-preferences", strings.NewReader(test.body))
			request = request.WithContext(principal.WithActor(request.Context(), principal.Principal{Type: principal.PrincipalAgent, ID: "agent:reviewer"}))
			recorder := httptest.NewRecorder()
			handlers := NewHandlers(&Service{})
			if test.method == http.MethodGet {
				handlers.ListApprovalPreferences(recorder, request)
			} else {
				handlers.UpdateApprovalPreference(recorder, request)
			}
			if test.status == http.StatusForbidden && !strings.Contains(recorder.Body.String(), "permission_denied") {
				t.Fatalf("missing permission denial: %s", recorder.Body.String())
			}
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}
}
