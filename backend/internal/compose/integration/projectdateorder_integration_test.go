// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
)

// A project's target end date may not fall before its start date. Both doors
// refuse it on create, and on a patch that moves either date against the one
// stored. The refusal names the date the caller sent.
func TestATargetEndBeforeTheStartIsRefusedOnEveryDoor(t *testing.T) {
	d := newTwoDoors(t, "project-date-order", "read", "write")
	company := anIDIn(t, d.restStatus(t, "POST", "/v1/companies",
		map[string]any{"display_name": "Order GmbH", "source": "manual"}, http.StatusCreated))
	project := func(fields map[string]any) map[string]any {
		fields["name"], fields["company_id"], fields["source"] = "Rollout", company, "manual"
		return fields
	}

	refused := d.restStatus(t, "POST", "/v1/projects",
		project(map[string]any{"started_at": "2026-12-01", "target_end_date": "2026-01-01"}), http.StatusUnprocessableEntity)
	assertDateOrderRefusal(t, "create", string(refused), "target_end_date")
	if refusal := d.toolRefused(t, "create_record", map[string]any{
		"record_type": "project",
		"fields":      project(map[string]any{"started_at": "2026-12-01", "target_end_date": "2026-01-01"}),
	}); !strings.Contains(refusal, "target_end_date") {
		t.Errorf("the tool refused an inverted pair as %q, want it to name target_end_date", refusal)
	}

	id := anIDIn(t, d.restStatus(t, "POST", "/v1/projects",
		project(map[string]any{"started_at": "2026-03-01", "target_end_date": "2026-06-30"}), http.StatusCreated))
	refused = d.restStatus(t, "PATCH", "/v1/projects/"+id,
		map[string]any{"target_end_date": "2026-02-01"}, http.StatusUnprocessableEntity)
	assertDateOrderRefusal(t, "a target end moved before the stored start", string(refused), "target_end_date")
	refused = d.restStatus(t, "PATCH", "/v1/projects/"+id,
		map[string]any{"started_at": "2026-07-01"}, http.StatusUnprocessableEntity)
	assertDateOrderRefusal(t, "a start moved past the stored target end", string(refused), "started_at")
	if refusal := d.toolRefused(t, "update_record", map[string]any{
		"record_type": "project", "id": id,
		"fields": map[string]any{"target_end_date": "2026-02-01"},
	}); !strings.Contains(refusal, "target_end_date") {
		t.Errorf("the tool refused a target end before the start as %q, want it to name target_end_date", refusal)
	}

	// The same day on both ends is a one-day project, and clearing the start
	// leaves no order to break.
	d.restStatus(t, "PATCH", "/v1/projects/"+id, map[string]any{"target_end_date": "2026-03-01"}, http.StatusOK)
	d.restStatus(t, "PATCH", "/v1/projects/"+id, map[string]any{"started_at": nil, "target_end_date": "2026-01-15"}, http.StatusOK)
}

func assertDateOrderRefusal(t *testing.T, what, body, field string) {
	t.Helper()
	if !strings.Contains(body, `"field":"`+field+`"`) || !strings.Contains(body, `"code":"invalid_date_range"`) {
		t.Errorf("%s answered %s, want invalid_date_range on %s", what, body, field)
	}
}
