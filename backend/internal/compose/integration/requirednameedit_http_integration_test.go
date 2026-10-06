// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A required name is required on an edit as it is on a create: an edit that
// blanks it leaves a record that is a blank row in every list and picker.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAnEditCannotBlankARecordsRequiredNameOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)

	records := []struct {
		kind, path, field string
		create            AnyMap
	}{
		{"contact", "/v1/contacts", "full_name", AnyMap{"full_name": "Ada Lovelace", "source": "manual"}},
		{"company", "/v1/companies", "display_name", AnyMap{"display_name": "Northwind", "source": "manual"}},
		{"deal", "/v1/deals", "name", AnyMap{"name": "Rollout", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual"}},
		{"lead", "/v1/leads", "full_name", AnyMap{"full_name": "Lee Lead", "email": "lee@lead.test", "source": "manual"}},
		{"view", "/v1/views", "name", AnyMap{"resource": "contacts", "name": "Mine", "query": AnyMap{"columns": []any{"full_name"}}}},
		{"product", "/v1/products", "name", AnyMap{"name": "Seat", "unit_price_minor": 1000, "currency": "EUR", "source": "manual"}},
	}
	for _, rec := range records {
		t.Run(rec.kind, func(t *testing.T) {
			id := createdID(t, e, rec.path, rec.create)
			original := rec.create[rec.field]
			for label, value := range map[string]any{"empty": "", "spaces": "   ", "null": nil} {
				var problem projectProblem
				status := e.Call(t, "PATCH", rec.path+"/"+id, AnyMap{rec.field: value}, nil, &problem)
				if status != http.StatusUnprocessableEntity && status != http.StatusBadRequest {
					t.Errorf("PATCH %s %s=%s → %d, want a 4xx refusal", rec.kind, rec.field, label, status)
				}
			}
			var read AnyMap
			if status := e.Call(t, "GET", rec.path+"/"+id, nil, nil, &read); status != http.StatusOK {
				t.Fatalf("GET %s → %d", rec.kind, status)
			}
			if read[rec.field] != original {
				t.Errorf("%s %s = %v after the refused edits, want it kept as %v", rec.kind, rec.field, read[rec.field], original)
			}
		})
	}
}
