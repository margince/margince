// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A required name is required on an edit as it is on a create: an edit that
// blanks it leaves a record that is a blank row in every list and picker.

import (
	"fmt"
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
				status := e.Call(t, "PATCH", rec.path+"/"+id, AnyMap{rec.field: value}, nil, nil)
				if status != http.StatusUnprocessableEntity {
					t.Errorf("PATCH %s %s=%s → %d, want 422", rec.kind, rec.field, label, status)
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

// The same rule on the records whose names were left out of the first pass:
// an offer template, a knowledge corpus and a lead's name when one is sent.
func TestASiblingRecordsRequiredNameIsRefusedBlankOnCreateAndEdit(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	t.Run("offer template edit", func(t *testing.T) {
		id := createdID(t, e, "/v1/offer-templates", AnyMap{"name": "Standard", "layout": AnyMap{}})
		var read struct {
			Version int64 `json:"version"`
		}
		if status := e.Call(t, "GET", "/v1/offer-templates/"+id, nil, nil, &read); status != http.StatusOK {
			t.Fatalf("GET template → %d", status)
		}
		status := e.Call(t, "PUT", "/v1/offer-templates/"+id,
			AnyMap{"name": "   ", "locale": "en-GB", "is_default": false, "layout": AnyMap{}},
			map[string]string{"If-Match": fmt.Sprint(read.Version)}, nil)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("PUT a template named with spaces → %d, want 422", status)
		}
	})

	t.Run("corpus create", func(t *testing.T) {
		status := e.Call(t, "POST", "/v1/knowledge/corpora", AnyMap{"name": "   ", "topic_statement": "Pricing"}, nil, nil)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("POST a corpus named with spaces → %d, want 422", status)
		}
	})

	t.Run("corpus edit", func(t *testing.T) {
		id := createdID(t, e, "/v1/knowledge/corpora", AnyMap{"name": "Pricing", "topic_statement": "Pricing"})
		status := e.Call(t, "PATCH", "/v1/knowledge/corpora/"+id, AnyMap{"name": "   "}, nil, nil)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("PATCH a corpus name to spaces → %d, want 422", status)
		}
	})

	t.Run("lead create", func(t *testing.T) {
		status := e.Call(t, "POST", "/v1/leads", AnyMap{"full_name": "   ", "email": "blank@lead.test", "source": "manual"}, nil, nil)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("POST a lead named with spaces → %d, want 422", status)
		}
	})
}
