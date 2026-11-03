// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

const individualPerformanceKey = "individual_performance"

func readIndividual(t *testing.T, e *apptest.AppEnv) bool {
	t.Helper()
	var settings map[string]json.RawMessage
	if status := e.Call(t, http.MethodGet, "/v1/installation/settings", nil, nil, &settings); status != http.StatusOK {
		t.Fatalf("GET /v1/installation/settings = %d, want 200", status)
	}
	var on bool
	if err := json.Unmarshal(settings[individualPerformanceKey], &on); err != nil {
		t.Fatalf("GET /v1/installation/settings carries %s = %s, want a boolean: %v", individualPerformanceKey, settings[individualPerformanceKey], err)
	}
	return on
}

// runWinLossByOwner asks the HTTP door for a breakdown by owner and answers
// the status and problem code.
func runWinLossByOwner(t *testing.T, e *apptest.AppEnv) (int, string) {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	body := AnyMap{
		"group_by":   []string{"owner_id", "currency"},
		"aggregates": []AnyMap{{"fn": "sum", "field": "amount_minor", "as": "won"}},
	}
	status := e.Call(t, http.MethodPost, "/v1/reports/win-loss", body, nil, &problem)
	return status, problem.Code
}

// The switch is on by default, and an admin's change is audited. The HTTP door
// applies it to the next request. A breakdown by owner is refused with a code
// the screen explains, and answers again once the switch is back on.
func TestIndividualPerformanceIsAnAuditedSwitchTheNextRequestObeys(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	if !readIndividual(t, e) {
		t.Fatal("an untouched installation shows no individual performance, want the default on")
	}
	if status, code := runWinLossByOwner(t, e); status != http.StatusOK {
		t.Fatalf("a breakdown by owner with the switch on = %d %q, want 200", status, code)
	}

	before := callMessengerAudits(t, e, individualPerformanceKey)
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: false}, nil, nil); status != http.StatusOK {
		t.Fatalf("admin PATCH {%s: false} = %d, want 200", individualPerformanceKey, status)
	}
	if after := callMessengerAudits(t, e, individualPerformanceKey); after != before+1 {
		t.Errorf("PATCH {%s: false} wrote %d audit rows for it, want 1", individualPerformanceKey, after-before)
	}
	if readIndividual(t, e) {
		t.Fatal("after PATCH false the installation still shows individual performance")
	}
	if status, code := runWinLossByOwner(t, e); status != http.StatusForbidden || code != "individual_performance_off" {
		t.Errorf("a breakdown by owner with the switch off = %d %q, want 403 individual_performance_off", status, code)
	}

	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: true}, nil, nil); status != http.StatusOK {
		t.Fatalf("admin PATCH {%s: true} = %d, want 200", individualPerformanceKey, status)
	}
	if status, code := runWinLossByOwner(t, e); status != http.StatusOK {
		t.Errorf("a breakdown by owner with the switch back on = %d %q, want 200", status, code)
	}

	demoteToRep(t, e)
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: false}, nil, nil); status != http.StatusForbidden {
		t.Errorf("rep PATCH {%s: false} = %d, want 403", individualPerformanceKey, status)
	}
}
