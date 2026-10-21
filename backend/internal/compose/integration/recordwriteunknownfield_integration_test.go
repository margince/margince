// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A record write names only fields the record has. The schemas admit extra
// keys so a custom field can ride beside the standard ones, but a key that is
// neither a standard field nor a cf_ key is a mistake, and a 200 that changed
// nothing would tell the caller the write was done.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func refusesUnknownField(problem problemBody) bool {
	for _, refusal := range problem.Details.Errors {
		if refusal.Code == "unknown_field" {
			return true
		}
	}
	return false
}

func TestARecordWriteNamingNoFieldIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)

	contact := createAndID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Typo Contact"})
	company := createAndID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Typo Co"})
	lead := createAndID(t, e, "/v1/leads", AnyMap{"full_name": "Typo Lead", "source": "manual"})
	deal := createAndID(t, e, "/v1/deals", AnyMap{
		"name": "Typo deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"source": "manual", "amount_minor": 1000, "currency": "EUR",
	})
	project := createAndID(t, e, "/v1/projects", AnyMap{"name": "Typo project", "company_id": company, "source": "manual"})

	edits := []struct {
		name, path string
		body       AnyMap
	}{
		{"contact", "/v1/contacts/" + contact, AnyMap{"ful_name": "x"}},
		{"company", "/v1/companies/" + company, AnyMap{"display_nam": "x"}},
		{"lead", "/v1/leads/" + lead, AnyMap{"ful_name": "x"}},
		{"deal typo", "/v1/deals/" + deal, AnyMap{"amout_minor": 999}},
		{"deal field of another route", "/v1/deals/" + deal, AnyMap{"stage_id": stages.Open}},
		{"deal wrong case", "/v1/deals/" + deal, AnyMap{"Amount_Minor": 999}},
		{"project", "/v1/projects/" + project, AnyMap{"nam": "x"}},
	}
	for _, edit := range edits {
		t.Run("edit "+edit.name, func(t *testing.T) {
			var problem problemBody
			status := e.Call(t, "PATCH", edit.path, edit.body, map[string]string{"If-Match": "1"}, &problem)
			if status != http.StatusUnprocessableEntity || !refusesUnknownField(problem) {
				t.Fatalf("PATCH %s %v → %d %+v, want 422 unknown_field", edit.path, edit.body, status, problem)
			}
		})
	}

	creates := []struct {
		name, path string
		body       AnyMap
	}{
		{"contact", "/v1/contacts", AnyMap{"source": "manual", "full_name": "x", "emial": "a@b.example"}},
		{"company", "/v1/companies", AnyMap{"source": "manual", "display_name": "x", "websit": "x"}},
		{"lead", "/v1/leads", AnyMap{"source": "manual", "full_name": "x", "emial": "a@b.example"}},
		{"deal", "/v1/deals", AnyMap{
			"name": "x", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
			"source": "manual", "amount_minor": 5, "currency": "EUR", "expected_arr_minr": 5,
		}},
	}
	for _, create := range creates {
		t.Run("create "+create.name, func(t *testing.T) {
			var problem problemBody
			status := e.Call(t, "POST", create.path, create.body, nil, &problem)
			if status != http.StatusUnprocessableEntity || !refusesUnknownField(problem) {
				t.Fatalf("POST %s %v → %d %+v, want 422 unknown_field", create.path, create.body, status, problem)
			}
		})
	}

	var read struct {
		AmountMinor int64 `json:"amount_minor"`
		Version     int64 `json:"version"`
	}
	if status := e.Call(t, "GET", "/v1/deals/"+deal, nil, nil, &read); status != http.StatusOK || read.AmountMinor != 1000 || read.Version != 1 {
		t.Fatalf("the refused edits left the deal at %+v (status %d), want amount 1000 at version 1", read, status)
	}
}
