// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The tag verbs, create_task and leads through both bulk doors — the /v1/bulk
// routes as a signed-in human and bulk_update_records as an agent — and a
// Shortlist's export. Every bulk assertion runs on both doors, because they are
// one engine.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedDoorTag coins a tag through the tag endpoint.
func seedDoorTag(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var tag struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/tags", AnyMap{"name": "Door tag " + ids.NewV7().String()[:8]}, nil, &tag); status != http.StatusCreated {
		t.Fatalf("seeding a tag → %d", status)
	}
	return tag.ID
}

func tagContacts(verb, tag string, items []bulkItemDTO) AnyMap {
	return AnyMap{"record_type": "contact", "verb": verb, "tag_id": tag, "items": items}
}

func taskContacts(items []bulkItemDTO) AnyMap {
	return AnyMap{
		"record_type": "contact", "verb": "create_task", "items": items,
		"task": AnyMap{"subject": "Send the renewal quote"},
	}
}

func TestBothDoorsTagASelectionAndTheUndoTakesOffOnlyWhatTheyAdded(t *testing.T) {
	e, doors := bulkUndoApp(t, "bulk-tag")
	tag := seedDoorTag(t, e)
	for _, door := range doors {
		items := seedBulkContacts(t, e, 3)
		if status := e.Call(t, "POST", "/v1/tags/"+tag+"/apply",
			AnyMap{"entity_type": "contact", "entity_id": items[0].ID}, nil, nil); status != http.StatusCreated {
			t.Fatalf("%s: tagging one contact beforehand → %d", door.name, status)
		}
		out, refusal := door.execute(t, tagContacts("add_tag", tag, items), "")
		if refusal != "" || out.Changed != 2 || len(out.Skipped) != 1 ||
			out.Skipped[0].ID != items[0].ID || out.Skipped[0].Reason != "no_change" {
			t.Fatalf("%s: tagging three → %+v, refused %q; want two tagged and the tagged one a no_change", door.name, out, refusal)
		}
		if n := countOwned(t, e, `SELECT count(*) FROM audit_log WHERE batch_id = $1 AND entity_type = 'tag'`, out.BatchID); n != 2 {
			t.Errorf("%s: %d tag audit rows carry the batch id, want two", door.name, n)
		}
		undone, refusal := door.undo(t, out.BatchID, "")
		if refusal != "" || undone.Changed != 2 {
			t.Fatalf("%s: undo → %+v, refused %q", door.name, undone, refusal)
		}
		if n := countOwned(t, e, `SELECT count(*) FROM taggable WHERE tag_id = $1 AND entity_id::text = ANY($2)`,
			tag, []string{items[0].ID, items[1].ID, items[2].ID}); n != 1 {
			t.Errorf("%s: after the undo %d of the three carry the tag, want only the one tagged before", door.name, n)
		}
	}
}

func TestBothDoorsNeedATokenToTagOrFileTasksUnderMoreThanTen(t *testing.T) {
	e, _, doors := bulkDoorsApp(t, "bulk-tag-token", agentvolume.Limits{})
	tag := seedDoorTag(t, e)
	for _, door := range doors {
		for _, change := range []AnyMap{
			tagContacts("add_tag", tag, seedBulkContacts(t, e, 11)),
			taskContacts(seedBulkContacts(t, e, 11)),
		} {
			verb := change["verb"]
			if _, refusal := door.execute(t, change, ""); !strings.Contains(refusal, "confirm_token") {
				t.Fatalf("%s %s: eleven without a token → %q, want a refusal naming confirm_token", door.name, verb, refusal)
			}
			preview, refusal := door.preview(t, change)
			if refusal != "" || !preview.RequiresConfirmation || preview.Count != 11 {
				t.Fatalf("%s %s: preview → %+v, refused %q", door.name, verb, preview, refusal)
			}
			change["confirm_token"] = preview.ConfirmToken
			if out, refusal := door.execute(t, change, ""); refusal != "" || out.Changed != 11 {
				t.Errorf("%s %s: executing with the token → %+v, refused %q", door.name, verb, out, refusal)
			}
		}
	}
}

func TestBothDoorsFileATaskUnderEachRecordAndTheUndoArchivesThem(t *testing.T) {
	e, doors := bulkUndoApp(t, "bulk-task")
	for _, door := range doors {
		items := seedBulkContacts(t, e, 3)
		out, refusal := door.execute(t, taskContacts(items), "")
		if refusal != "" || out.Changed != 3 {
			t.Fatalf("%s: filing three tasks → %+v, refused %q", door.name, out, refusal)
		}
		live := `SELECT count(*) FROM activity WHERE kind = 'task' AND archived_at IS NULL
			AND id IN (SELECT entity_id FROM audit_log WHERE batch_id = $1 AND entity_type = 'activity')`
		if n := countOwned(t, e, live, out.BatchID); n != 3 {
			t.Fatalf("%s: %d live tasks trace to the batch, want three", door.name, n)
		}
		undone, refusal := door.undo(t, out.BatchID, "")
		if refusal != "" || undone.Changed != 3 {
			t.Fatalf("%s: undo → %+v, refused %q", door.name, undone, refusal)
		}
		if n := countOwned(t, e, live, out.BatchID); n != 0 {
			t.Errorf("%s: %d of the batch's tasks are still live after the undo", door.name, n)
		}
		if n := countOwned(t, e, `SELECT count(*) FROM audit_log WHERE batch_id = $1 AND action = 'archive'`, undone.BatchID); n != 3 {
			t.Errorf("%s: %d archive audit rows carry the undo's batch id, want three", door.name, n)
		}
	}
}

func TestBothDoorsReassignLeadsAndRefuseToArchiveThem(t *testing.T) {
	e, _, doors := bulkDoorsApp(t, "bulk-leads", agentvolume.Limits{})
	colleague := seedColleague(t, e)
	for _, door := range doors {
		leads := make([]bulkItemDTO, 0, 2)
		for range 2 {
			var lead bulkItemDTO
			if status := e.Call(t, "POST", "/v1/leads", AnyMap{
				"full_name": "Bulk lead " + ids.NewV7().String(), "source": "manual",
			}, nil, &lead); status != http.StatusCreated {
				t.Fatalf("seeding a lead → %d", status)
			}
			leads = append(leads, lead)
		}
		moved, refusal := door.execute(t, AnyMap{"record_type": "lead", "verb": "reassign_owner", "owner_id": colleague, "items": leads}, "")
		if refusal != "" || moved.Changed != 2 {
			t.Fatalf("%s: reassigning two leads → %+v, refused %q", door.name, moved, refusal)
		}
		if _, refusal := door.execute(t, AnyMap{"record_type": "lead", "verb": "archive", "items": leads}, ""); !strings.Contains(refusal, "archive") {
			t.Errorf("%s: archiving leads → %q, want a refusal saying a lead has no archive", door.name, refusal)
		}
	}
}

func TestAShortlistExportsItsMembersAndNothingElse(t *testing.T) {
	e, _ := listsApp(t, true)
	var list listDTO
	if status := e.Call(t, "POST", "/v1/lists", AnyMap{
		"name": "Renewal calls", "entity_type": "contact", "list_type": "static",
	}, nil, &list); status != http.StatusCreated {
		t.Fatalf("create Shortlist → %d", status)
	}
	contacts := seedBulkContacts(t, e, 3)
	for _, member := range contacts[:2] {
		if status := e.Call(t, "POST", "/v1/lists/"+list.ID+"/members",
			AnyMap{"entity_type": "contact", "entity_id": member.ID}, nil, nil); status >= http.StatusBadRequest {
			t.Fatalf("adding a member → %d", status)
		}
	}
	rows := exportCSV(t, e, AnyMap{"list_id": list.ID, "format": "csv"})
	if len(rows) != 3 {
		t.Fatalf("the export has %d rows, want a header and the two members", len(rows))
	}
	body := fmt.Sprint(rows)
	if strings.Contains(body, contacts[2].ID) {
		t.Error("the export carries a contact that is not on the Shortlist")
	}
}
