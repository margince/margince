// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func callCaptureHealth(p principal.Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/capture-health", nil)
	rec := httptest.NewRecorder()
	captureHealthHandlers{}.GetCaptureHealth(rec, req.WithContext(principal.WithActor(req.Context(), p)))
	return rec
}

// A passport minted by an admin satisfies every object grant, so the human
// rung is what keeps an agent off a page about everyone's mailboxes.
func TestCaptureHealthRefusesAnAgentHoldingTheGrant(t *testing.T) {
	rec := callCaptureHealth(principal.Principal{
		Type: principal.PrincipalAgent,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"job_health": {Read: true}},
		},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("an agent got %d, want 403", rec.Code)
	}
}

func TestCaptureHealthRefusesAHumanWithoutTheJobHealthGrant(t *testing.T) {
	rec := callCaptureHealth(principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"contact": {Read: true}},
		},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("a human without job_health got %d, want 403", rec.Code)
	}
}
