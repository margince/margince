// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A filter value outside the enum the contract declares is a mistake to name,
// never an empty answer. The same request with a real value still answers.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAFilterValueOutsideItsEnumIsRefusedOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	for _, tc := range []struct{ path, ok, bad string }{
		{"/v1/leads", "status=new", "status=bogus"},
		{"/v1/leads", "sla_state=breached", "sla_state=bogus"},
		{"/v1/dedupe/candidates", "status=open", "status=bogus"},
		{"/v1/dedupe/candidates", "entity_type=contact", "entity_type=bogus"},
		{"/v1/activities", "kind=note", "kind=bogus"},
		{"/v1/relationships", "kind=employment", "kind=bogus"},
		{"/v1/approvals", "status=pending", "status=bogus"},
		{"/v1/approvals", "status=pending", "status=expired"},
		// The movement read needs two snapshots before any reading is judged, so
		// only the refusal is asserted for it.
		{"/v1/forecast/movement", "", "reading=zz_not_a_reading"},
	} {
		t.Run(tc.path+"?"+tc.bad, func(t *testing.T) {
			if status := e.Call(t, "GET", tc.path+"?"+tc.bad, nil, nil, nil); status != http.StatusUnprocessableEntity {
				t.Errorf("GET %s?%s → %d, want 422", tc.path, tc.bad, status)
			}
			// A real value never reaches the refusal, whatever else the route answers.
			if tc.ok == "" {
				return
			}
			if status := e.Call(t, "GET", tc.path+"?"+tc.ok, nil, nil, nil); status == http.StatusUnprocessableEntity {
				t.Errorf("GET %s?%s → 422, a value in the enum was refused", tc.path, tc.ok)
			}
		})
	}
}

func TestHalfAnEntityPairOnTheTimelineIsRefusedOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	const someID = "018f3a1b-0000-7000-8000-000000000010"
	for _, query := range []string{"entity_id=" + someID, "entity_type=contact"} {
		if status := e.Call(t, "GET", "/v1/activities?"+query, nil, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("GET /v1/activities?%s → %d, want 422", query, status)
		}
	}
}
