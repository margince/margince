// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The saves with no If-Match, which no route census reaches: each sent twice
// with the same values writes history once.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// assertRepeatWritesNothing sends one write twice and checks the second left
// no audit row and no event anywhere.
func assertRepeatWritesNothing(t *testing.T, e *apptest.AppEnv, method, path string, body AnyMap, want int) {
	t.Helper()
	if status := e.Call(t, method, path, body, nil, nil); status != want {
		t.Fatalf("%s %s -> %d, want %d", method, path, status, want)
	}
	history := func() [2]int {
		t.Helper()
		var counts [2]int
		if err := e.Owner.QueryRow(t.Context(),
			`SELECT (SELECT count(*) FROM audit_log), (SELECT count(*) FROM event_outbox)`).Scan(&counts[0], &counts[1]); err != nil {
			t.Fatal(err)
		}
		return counts
	}
	before := history()
	if status := e.Call(t, method, path, body, nil, nil); status != want {
		t.Fatalf("%s %s sent again -> %d, want %d", method, path, status, want)
	}
	if after := history(); after != before {
		t.Errorf("%s %s sent again wrote audit rows and events %v -> %v, want none", method, path, before, after)
	}
}

func TestASettingsSaveSentTwiceWritesHistoryOnce(t *testing.T) {
	e := setupRelationships(t)
	t.Run("project company", func(t *testing.T) {
		project := createdID(t, e.AppEnv, "/v1/projects", AnyMap{"name": "Twice", "company_id": e.companyID, "source": "manual"})
		partner := createdID(t, e.AppEnv, "/v1/companies", AnyMap{"display_name": "Partner Twice", "source": "manual"})
		assertRepeatWritesNothing(t, e.AppEnv, "PUT", "/v1/projects/"+project+"/companies",
			AnyMap{"company_id": partner, "role": "implementation partner"}, http.StatusOK)
	})
	t.Run("domain admission", func(t *testing.T) {
		assertRepeatWritesNothing(t, e.AppEnv, "PUT", "/v1/capture/blocked-domains",
			AnyMap{"domain": "twice.test", "admission": "admitted", "reason": "They became a client."}, http.StatusOK)
	})
	t.Run("fx rate", func(t *testing.T) {
		assertRepeatWritesNothing(t, e.AppEnv, "POST", "/v1/fx-rates", AnyMap{"from_currency": "CHF", "rate": "1.05"}, http.StatusCreated)
	})
	t.Run("model rate", func(t *testing.T) {
		price := AnyMap{
			"provider": "anthropic", "model_id": "twice-model", "input_per_mtok": "3", "output_per_mtok": "15",
			"cache_read_per_mtok": "0.3", "cache_write_per_mtok": "3.75",
		}
		mustCall(t, e.AppEnv, "POST", "/v1/ai-model-rates", price, http.StatusCreated, nil)
		// The price table has no version, so the row's own xmin is what shows a rewrite.
		rowVersion := func() string {
			var xmin string
			if err := e.Owner.QueryRow(t.Context(),
				`SELECT xmin::text FROM ai_model_rate WHERE model_id = 'twice-model'`).Scan(&xmin); err != nil {
				t.Fatal(err)
			}
			return xmin
		}
		stored := rowVersion()
		assertRepeatWritesNothing(t, e.AppEnv, "POST", "/v1/ai-model-rates", price, http.StatusCreated)
		if again := rowVersion(); again != stored {
			t.Errorf("the price sent again rewrote its row: xmin %s -> %s", stored, again)
		}
	})
	t.Run("working hours", func(t *testing.T) {
		assertRepeatWritesNothing(t, e.AppEnv, "PUT", "/v1/me/working-hours",
			AnyMap{"start_time": "08:30", "end_time": "16:30", "days": []int{1, 2, 3, 4}}, http.StatusOK)
	})
	t.Run("onboarding state", func(t *testing.T) {
		// Confirm is a step the creator may save before the company is complete.
		// The version each save sends is the one the last answer carried.
		var state struct {
			Version int64 `json:"version"`
		}
		draft := AnyMap{"step": "confirm", "source_mode": "manual", "company_draft": AnyMap{}, "selected_fact_keys": []string{}}
		if status := e.Call(t, "GET", "/v1/onboarding/state", nil, nil, &state); status == http.StatusNotFound {
			draft["expected_version"] = 0
			mustCall(t, e.AppEnv, "PUT", "/v1/onboarding/state", draft, http.StatusOK, &state)
		}
		draft["expected_version"] = state.Version
		mustCall(t, e.AppEnv, "PUT", "/v1/onboarding/state", draft, http.StatusOK, &state)
		draft["expected_version"] = state.Version
		assertRepeatWritesNothing(t, e.AppEnv, "PUT", "/v1/onboarding/state", draft, http.StatusOK)
	})
	t.Run("review template questions as read", func(t *testing.T) {
		var templates struct {
			Data []struct {
				ID        string          `json:"id"`
				Version   int64           `json:"version"`
				Questions json.RawMessage `json:"questions"`
			} `json:"data"`
		}
		mustCall(t, e.AppEnv, "GET", "/v1/activity-review-templates", nil, http.StatusOK, &templates)
		if len(templates.Data) == 0 {
			t.Fatal("no review template is seeded")
		}
		template := templates.Data[0]
		assertRepeatWritesNothing(t, e.AppEnv, "PATCH", "/v1/activity-review-templates/"+template.ID,
			AnyMap{"version": template.Version, "questions": template.Questions}, http.StatusOK)
	})
	t.Run("offer line", func(t *testing.T) {
		offerID := offerValidUntil(t, e.AppEnv, offerFixture(t, e.AppEnv), "2099-12-31")
		var offer struct {
			LineItems []struct {
				ID string `json:"id"`
			} `json:"line_items"`
		}
		mustCall(t, e.AppEnv, "GET", "/v1/offers/"+offerID, nil, http.StatusOK, &offer)
		if len(offer.LineItems) == 0 {
			t.Fatal("the offer has no line")
		}
		assertRepeatWritesNothing(t, e.AppEnv, "PATCH", "/v1/offers/"+offerID+"/line-items/"+offer.LineItems[0].ID,
			AnyMap{"description": "Retainer, monthly"}, http.StatusOK)
	})
}

func TestACustomFieldOptionListSentTwiceWritesHistoryOnce(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(SchemaPool(t)))
	e.BootstrapWorkspace(t)
	field := createdID(t, e, "/v1/custom-fields", AnyMap{
		"object": "deal", "label": "Tier", "type": "picklist", "source": "manual", "options": []string{"Gold", "Silver"},
	})
	assertRepeatWritesNothing(t, e, "PATCH", "/v1/custom-fields/"+field+"/options",
		AnyMap{"options": []string{"Gold", "Silver", "Bronze"}}, http.StatusOK)
}
