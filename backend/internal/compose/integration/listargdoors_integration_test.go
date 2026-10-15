// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
)

// A list's text query narrows list_records as it narrows the record type's
// list route, together with its filters.
func TestListRecordsTakesTheListRoutesTextQuery(t *testing.T) {
	d := newTwoDoors(t, "list-q-doors", "read", "write")
	wanted := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Zelda Quorn"})
	createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Otto Brandt"})

	sameDoorRecords(t, "contacts matching Quorn",
		d.rest(t, "GET", "/v1/contacts?q=Quorn", nil),
		d.tool(t, "list_records", map[string]any{"record_type": "contact", "q": "Quorn"}),
		wanted)
}

// carriedTag is one word with the count both doors give when asked for it.
type carriedTag struct {
	Name      string `json:"name"`
	CarriedBy *int   `json:"carried_by"`
}

// list_tags counts each word's records when asked, as GET /tags does, and
// list_pipelines lists a retired pipeline when asked, as GET /pipelines does.
func TestReferenceListsTakeTheirRoutesWidenings(t *testing.T) {
	d := newTwoDoors(t, "widen-doors", "read", "write")
	contact := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Ada Tagged"})
	tag := createdRecord(t, d.e, "/v1/tags", AnyMap{"name": "Carried"})
	d.tool(t, "apply_tag", map[string]any{"tag_id": tag, "record_type": "contact", "record_id": contact})

	restTags := decodedAs[struct {
		Data []carriedTag `json:"data"`
	}](t, d.rest(t, "GET", "/v1/tags?with_carried_by=true", nil))
	toolTags := decodedAs[struct {
		Tags []carriedTag `json:"tags"`
	}](t, d.tool(t, "list_tags", map[string]any{"with_carried_by": true}))
	if len(restTags.Data) != 1 || len(toolTags.Tags) != 1 || restTags.Data[0].CarriedBy == nil || toolTags.Tags[0].CarriedBy == nil ||
		*restTags.Data[0].CarriedBy != 1 || *toolTags.Tags[0].CarriedBy != 1 {
		t.Errorf("the doors count the word's records as %+v and %+v, want one record on each", restTags.Data, toolTags.Tags)
	}

	retired := createdRecord(t, d.e, "/v1/pipelines", AnyMap{"name": "Retired", "stages": []AnyMap{{"name": "Scout", "position": 1}}})
	if status := d.e.Call(t, "DELETE", "/v1/pipelines/"+retired, nil, nil, nil); status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("retiring a pipeline → %d", status)
	}
	sameDoorRecords(t, "pipelines with the retired ones",
		listedPipelineIDs(t, d.rest(t, "GET", "/v1/pipelines?include_archived=true", nil)),
		listedPipelineIDs(t, d.tool(t, "list_pipelines", map[string]any{"include_archived": true})),
		retired)
}

// listedPipelineIDs reduces either door's pipeline list to its pipelines' ids,
// leaving out the stages each nests under them.
func listedPipelineIDs(t *testing.T, answer json.RawMessage) json.RawMessage {
	t.Helper()
	type pipeline struct {
		ID string `json:"id"`
	}
	listed := decodedAs[struct {
		Data      []pipeline `json:"data"`
		Pipelines []pipeline `json:"pipelines"`
	}](t, answer)
	out, err := json.Marshal(append(listed.Data, listed.Pipelines...))
	if err != nil {
		t.Fatalf("encoding pipeline ids: %v", err)
	}
	return out
}

// decodedAs reads an answer into the shape a check compares.
func decodedAs[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the answer does not decode: %v\n%s", err, raw)
	}
	return out
}
