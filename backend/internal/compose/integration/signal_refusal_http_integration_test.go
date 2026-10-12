// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A signal body the store refuses answers 422 naming the field to fix, and
// files nothing. A subject type outside the signal's set is the caller's
// mistake, so it must never read as a server fault.
func TestASignalTheStoreRefusesAnswers422NamingTheField(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Signal Refusals", "admin@signalrefusals.test", "Admin")

	refusals := []struct {
		name, field, code string
		body              AnyMap
	}{
		{
			name: "subject type outside the set", field: "entity_type", code: "invalid_entity_type",
			body: AnyMap{
				"kind": "risk", "summary": "x", "source": "manual",
				"entity_type": "task", "entity_id": ids.NewV7().String(),
			},
		},
		{
			name: "summary left out", field: "summary", code: "required",
			body: AnyMap{"kind": "risk", "summary": "", "source": "manual"},
		},
	}
	for _, refusal := range refusals {
		t.Run(refusal.name, func(t *testing.T) {
			var problem struct {
				Details struct {
					Errors []struct {
						Field string `json:"field"`
						Code  string `json:"code"`
					} `json:"errors"`
				} `json:"details"`
			}
			status := e.Call(t, http.MethodPost, "/v1/signals", refusal.body, nil, &problem)
			named := len(problem.Details.Errors) == 1 &&
				problem.Details.Errors[0].Field == refusal.field && problem.Details.Errors[0].Code == refusal.code
			if status != http.StatusUnprocessableEntity || !named {
				t.Fatalf("POST /v1/signals → %d %+v, want 422 %s on %s", status, problem, refusal.code, refusal.field)
			}
		})
	}

	var page struct {
		Data []map[string]any `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/signals", nil, nil, &page); status != http.StatusOK || len(page.Data) != 0 {
		t.Fatalf("GET /v1/signals → %d with %d rows, want 200 and nothing filed by a refused body", status, len(page.Data))
	}
}
