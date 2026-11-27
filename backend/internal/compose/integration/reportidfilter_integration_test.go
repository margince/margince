// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A malformed id in a report filter is refused by the engine, before the
// statement binds it. Bound, Postgres refuses it as a database fault whose
// answer is value_wrong_type and whose cause the server logs as an ERROR.
func TestAMalformedIDFilterIsRefusedBeforeTheReportStatementRuns(t *testing.T) {
	t.Parallel()
	d := newTwoDoors(t, "report-id-filter", "read")

	for _, filter := range []string{"pipeline_id", "owner_id", "company_id"} {
		var fault struct{ Code, Detail string }
		body := d.restStatus(t, "POST", "/v1/reports/deals-by-stage",
			map[string]any{"filters": map[string]any{filter: "undefined"}}, http.StatusUnprocessableEntity)
		if err := json.Unmarshal(body, &fault); err != nil {
			t.Fatalf("decoding the refusal: %v\n%s", err, body)
		}
		if fault.Code != "report_field_not_allowed" || !strings.Contains(fault.Detail, `"`+filter+`"`) {
			t.Errorf("%s=undefined answered %s, want report_field_not_allowed naming the filter", filter, body)
		}
	}

	query := map[string]any{
		"entity": "deals-by-stage", "scope_kind": "workspace",
		"measures": []any{map[string]any{"fn": "count"}},
		"filters":  []any{map[string]any{"field": "pipeline_id", "op": "eq", "value": "undefined"}},
	}
	var refusal struct {
		Details struct{ Kind, Suggest string } `json:"details"`
	}
	if err := json.Unmarshal(d.restStatus(t, "POST", "/v1/analytics/query", query, http.StatusBadRequest), &refusal); err != nil {
		t.Fatalf("decoding the typed refusal: %v", err)
	}
	if refusal.Details.Kind != "invalid" || !strings.Contains(refusal.Details.Suggest, "8-4-4-4-12") {
		t.Errorf("the typed question's refusal %+v does not advise the id form", refusal.Details)
	}
}
