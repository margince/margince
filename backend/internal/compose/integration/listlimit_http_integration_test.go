// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The contract gives limit a minimum of 1, and the assembled router refuses a
// value below it instead of serving one row. Above the maximum the page is
// served at the maximum, as the contract says.
func TestAListPageSizeBelowOneIsRefusedThroughTheRouter(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	for _, limit := range []string{"0", "-1"} {
		var problem AnyMap
		if status := e.Call(t, "GET", "/v1/contacts?limit="+limit, nil, nil, &problem); status != http.StatusUnprocessableEntity {
			t.Fatalf("limit=%s → %d, want 422", limit, status)
		}
		raw, err := json.Marshal(problem)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "limit") || !strings.Contains(string(raw), "must be at least 1") {
			t.Errorf("limit=%s refusal %s does not name the parameter and its minimum", limit, raw)
		}
	}
	for _, limit := range []string{"1", "500"} {
		if status := e.Call(t, "GET", "/v1/contacts?limit="+limit, nil, nil, nil); status != http.StatusOK {
			t.Errorf("limit=%s → %d, want 200", limit, status)
		}
	}
}
