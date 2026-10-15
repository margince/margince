// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// The list controls a passport sends REST it sends list_records too. The two
// doors answer the same records: archived ones on request, one list's members,
// and the order a sort names.
func TestListRecordsTakesTheControlsTheRoutesTake(t *testing.T) {
	d := newTwoDoorsWith(t, "list-controls-doors", []compose.Option{compose.WithListsEnabled(true)}, "read")
	kept := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Ada Kept"})
	gone := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Bea Gone"})
	other := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Cy Other"})
	if status := d.e.Call(t, "DELETE", "/v1/contacts/"+gone, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archiving a contact → %d", status)
	}
	var list struct {
		ID string `json:"id"`
	}
	if status := d.e.Call(t, "POST", "/v1/lists", AnyMap{"name": "Renewals", "entity_type": "contact", "list_type": "static"},
		nil, &list); status != http.StatusCreated {
		t.Fatalf("creating a Shortlist → %d", status)
	}
	if status := d.e.Call(t, "POST", "/v1/lists/"+list.ID+"/members", AnyMap{"entity_type": "contact", "entity_id": kept},
		nil, nil); status >= http.StatusBadRequest {
		t.Fatalf("adding a member → %d", status)
	}

	sameDoorRecords(t, "with the archived",
		d.rest(t, "GET", "/v1/contacts?include_archived=true&limit=10", nil),
		d.tool(t, "list_records", map[string]any{"record_type": "contact", "limit": 10,
			"filters": map[string]any{"include_archived": "true"}}), kept, gone, other)
	sameDoorRecords(t, "one list's members",
		d.rest(t, "GET", "/v1/contacts?list_id="+list.ID+"&limit=10", nil),
		d.tool(t, "list_records", map[string]any{"record_type": "contact", "limit": 10,
			"filters": map[string]any{"list_id": list.ID}}), kept)
	sameDoorRecords(t, "sorted by name",
		d.rest(t, "GET", "/v1/contacts?sort=-full_name&limit=10", nil),
		d.tool(t, "list_records", map[string]any{"record_type": "contact", "limit": 10,
			"filters": map[string]any{"sort": "-full_name"}}), kept, other)
}
