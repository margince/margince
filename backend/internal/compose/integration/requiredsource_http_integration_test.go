// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The contract requires a source on a company, a contact and a deal, so a
// create that leaves it out or sends it blank is refused and names the field.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestACreateWithoutASourceIsRefusedNamingSourceOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	company := createAndID(t, e, "/v1/companies", AnyMap{"display_name": "Project Co", "source": "manual"})

	records := []struct {
		kind, path string
		body       AnyMap
	}{
		{"company", "/v1/companies", AnyMap{"display_name": "Northwind"}},
		{"contact", "/v1/contacts", AnyMap{"full_name": "Ada Lovelace"}},
		{"project", "/v1/projects", AnyMap{"name": "Rollout project", "company_id": company}},
		{"deal", "/v1/deals", AnyMap{"name": "Rollout", "pipeline_id": stages.PipelineID, "stage_id": stages.Open}},
	}
	for _, rec := range records {
		for label, value := range map[string]any{"missing": nil, "empty": "", "spaces": "   "} {
			t.Run(rec.kind+" "+label, func(t *testing.T) {
				body := AnyMap{}
				for k, v := range rec.body {
					body[k] = v
				}
				if label != "missing" {
					body["source"] = value
				}
				var problem struct {
					Details struct {
						Errors []struct {
							Field string `json:"field"`
						} `json:"errors"`
					} `json:"details"`
				}
				status := e.Call(t, "POST", rec.path, body, nil, &problem)
				if status != http.StatusUnprocessableEntity {
					t.Fatalf("POST %s with %s source → %d, want 422", rec.path, label, status)
				}
				if len(problem.Details.Errors) != 1 || problem.Details.Errors[0].Field != "source" {
					t.Errorf("POST %s with %s source named %+v, want the field source", rec.path, label, problem.Details.Errors)
				}
			})
		}
	}
}
