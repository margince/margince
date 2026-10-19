// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An activity custom field over both doors. A value logged over REST or the
// log_activity tool is read back and narrows the list. A key or value the
// catalog does not admit is refused rather than dropped.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// activityFieldDoors is two doors on a workspace whose admin added one picklist
// field to activities. It answers the doors and the field's column.
func activityFieldDoors(t *testing.T) (*twoDoors, string) {
	t.Helper()
	d := newTwoDoorsWith(t, "activity-custom-fields",
		[]compose.Option{compose.WithSchemaPool(SchemaPool(t))}, "read", "write")
	var field struct {
		ColumnName string `json:"column_name"`
	}
	if status := d.e.Call(t, "POST", "/v1/custom-fields", AnyMap{
		"object": "activity", "label": "Visit type", "type": "picklist",
		"options": []string{"on_site", "remote"}, "source": "manual",
	}, nil, &field); status != http.StatusCreated {
		t.Fatalf("create the activity field → %d", status)
	}
	return d, field.ColumnName
}

// activityRecord is what these tests read off an activity answer.
type activityRecord map[string]any

func decodeActivity(t *testing.T, raw json.RawMessage) activityRecord {
	t.Helper()
	var out activityRecord
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode activity %s: %v", raw, err)
	}
	return out
}

func (d *twoDoors) activityIDsWhere(t *testing.T, query string) map[string]bool {
	t.Helper()
	var list struct {
		Data []activityRecord `json:"data"`
	}
	if err := json.Unmarshal(d.restStatus(t, "GET", "/v1/activities?"+query, nil, http.StatusOK), &list); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, row := range list.Data {
		if id, ok := row["id"].(string); ok {
			out[id] = true
		}
	}
	return out
}

func TestAnActivityCustomFieldIsWrittenReadAndFilteredOverBothDoors(t *testing.T) {
	d, column := activityFieldDoors(t)

	logged := decodeActivity(t, d.restStatus(t, "POST", "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Plant walk-through", "source": "manual", column: "on_site",
	}, http.StatusCreated))
	if logged[column] != "on_site" {
		t.Fatalf("the create answer carries %v under %s, want on_site: %v", logged[column], column, logged)
	}
	restID, _ := logged["id"].(string)

	tool := decodeActivity(t, d.tool(t, "log_activity", map[string]any{
		"kind": "call", "subject": "Follow-up call", column: "remote",
	}))
	toolID, _ := tool["id"].(string)
	if toolID == "" {
		t.Fatalf("the tool answer names no activity: %v", tool)
	}

	for id, want := range map[string]string{restID: "on_site", toolID: "remote"} {
		got := decodeActivity(t, d.restStatus(t, "GET", "/v1/activities/"+id, nil, http.StatusOK))
		if got[column] != want {
			t.Errorf("GET /v1/activities/%s carries %v under %s, want %s", id, got[column], column, want)
		}
	}

	onSite := d.activityIDsWhere(t, column+"=on_site")
	if !onSite[restID] || onSite[toolID] {
		t.Errorf("filtering on %s=on_site listed %v, want the REST meeting and not the tool call", column, onSite)
	}
	if all := d.activityIDsWhere(t, ""); !all[restID] || !all[toolID] {
		t.Errorf("the unfiltered list lost one of the two activities: %v", all)
	}

	patched := decodeActivity(t, d.restStatus(t, "PATCH", "/v1/activities/"+restID, AnyMap{column: "remote"}, http.StatusOK))
	if patched[column] != "remote" {
		t.Errorf("the patch answer carries %v under %s, want remote", patched[column], column)
	}
	if onSite := d.activityIDsWhere(t, column+"=on_site"); onSite[restID] {
		t.Errorf("the patched meeting still filters as on_site: %v", onSite)
	}
	d.restStatus(t, "PATCH", "/v1/activities/"+restID, AnyMap{"subjcet": "typo"}, http.StatusUnprocessableEntity)
}

func TestAnActivityCreateRefusesWhatItCannotStore(t *testing.T) {
	d, column := activityFieldDoors(t)
	for name, body := range map[string]AnyMap{
		"a key that is not a custom field": {"subjcet": "typo"},
		"a field no workspace defined":     {"cf_nobody_defined": "x"},
		"a value of the wrong shape":       {column: true},
		"an option the picklist lacks":     {column: "by_post"},
	} {
		t.Run(name, func(t *testing.T) {
			body["kind"], body["subject"], body["source"] = "note", "Refused", "manual"
			d.restStatus(t, "POST", "/v1/activities", body, http.StatusUnprocessableEntity)
		})
	}
	d.restStatus(t, "GET", "/v1/activities?cf_nobody_defined=x", nil, http.StatusUnprocessableEntity)
}
