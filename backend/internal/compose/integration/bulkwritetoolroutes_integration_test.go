// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The writes that change several records or fold two into one:
// merge_records, bulk_update_records and change_lists. Each test asks both
// doors for the same change on two equal sets of records. It compares what each
// door answered or stored, and the audit and outbox rows it left.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

type sharedBulk struct {
	Changed int `json:"changed"`
	Skipped []struct {
		Reason string `json:"reason"`
	} `json:"skipped"`
}

// A tag put on two contacts in one bulk change is the same change on either
// door. The count is the same, nothing is skipped, and each record gets one
// audit row naming the agent.
func TestBulkUpdateRecordsLeavesTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "bulk-write-doors", "read", "write")
	tag := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Spring list", "color": "lime"})
	seed := func(names ...string) (items []map[string]any, records []string) {
		for _, name := range names {
			id := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": name, "source": "manual"})
			items = append(items, map[string]any{"id": id, "version": versionOf(t, d.storedContact(t, id))})
			records = append(records, id)
		}
		return items, records
	}
	restItems, viaREST := seed("Bulk Rest One", "Bulk Rest Two")
	toolItems, viaTool := seed("Bulk Tool One", "Bulk Tool Two")

	rest := d.restStatus(t, "POST", "/v1/bulk/execute", AnyMap{
		"record_type": "contact", "verb": "add_tag", "tag_id": tag, "items": restItems,
	}, http.StatusOK)
	tool := d.tool(t, "bulk_update_records", map[string]any{
		"mode": "execute", "record_type": "contact", "verb": "add_tag", "tag_id": tag, "items": toolItems,
	})
	changed := sameRecords[sharedBulk](t, "bulk", rest, tool)
	if changed.Changed != 2 || len(changed.Skipped) != 0 {
		t.Errorf("the bulk change changed %d and skipped %d, want 2 and none", changed.Changed, len(changed.Skipped))
	}
	for i := range viaREST {
		d.sameEffect(t, "bulk tag", viaREST[i], viaTool[i])
		if carried := d.tagsOn(t, viaTool[i]); len(carried) != 1 || carried[0] != tag {
			t.Errorf("contact %s carries %v after the bulk change, want the tag", viaTool[i], carried)
		}
	}
}

// A Shortlist made, renamed and given a member over either door is the same
// list with the same rows behind it.
func TestChangeListsLeavesTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoorsWith(t, "list-write-doors", []compose.Option{compose.WithListsEnabled(true)}, "read", "write")
	member := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Listed Lena", "source": "manual"})

	restMade := d.restStatus(t, "POST", "/v1/lists", AnyMap{
		"name": "Rest shortlist", "entity_type": "contact", "list_type": "static", "purpose": "Spring outreach",
	}, http.StatusCreated)
	toolMade := d.tool(t, "change_lists", map[string]any{
		"mode": "create", "name": "Tool shortlist", "entity_type": "contact", "list_type": "static", "purpose": "Spring outreach",
	})
	viaREST, viaTool := anIDIn(t, restMade), anIDIn(t, toolMade)
	stored := func(id string) json.RawMessage { return d.restStatus(t, "GET", "/v1/lists/"+id, nil, http.StatusOK) }
	made := sameRecords[struct {
		Purpose    *string `json:"purpose"`
		EntityType string  `json:"entity_type"`
		ListType   string  `json:"list_type"`
		Sharing    string  `json:"sharing"`
	}](t, "create", stored(viaREST), stored(viaTool))
	if made.ListType != "static" || made.Purpose == nil || *made.Purpose != "Spring outreach" {
		t.Errorf("the made list is %+v, want a static one with its purpose", made)
	}
	d.sameEffect(t, "create", viaREST, viaTool)

	d.restStatus(t, "POST", "/v1/lists/"+viaREST+"/members", AnyMap{"entity_type": "contact", "entity_id": member, "note": "Met in spring"}, http.StatusCreated)
	d.tool(t, "change_lists", map[string]any{
		"mode": "add_member", "list_id": viaTool, "entity_type": "contact", "record_id": member, "note": "Met in spring",
	})
	d.sameEffect(t, "add member", viaREST, viaTool)
}

// Folding one contact into another runs on either door: the source is archived
// and redirected to the survivor, and the rows left behind read alike.
func TestMergeRecordsLeavesTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "merge-write-doors", "read", "write")
	pair := func(source, survivor string) (string, string) {
		return createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": source, "source": "manual"}),
			createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": survivor, "source": "manual"})
	}
	restSource, restSurvivor := pair("Merge Rest Source", "Merge Rest Survivor")
	toolSource, toolSurvivor := pair("Merge Tool Source", "Merge Tool Survivor")

	d.restPinned(t, "POST", "/v1/contacts/"+restSource+"/merge", AnyMap{"target_id": restSurvivor},
		versionOf(t, d.storedContact(t, restSource)))
	d.tool(t, "merge_records", map[string]any{"record_type": "contact", "source_id": toolSource, "target_id": toolSurvivor})
	for _, pair := range [][2]string{{restSource, restSurvivor}, {toolSource, toolSurvivor}} {
		var read struct {
			Archived     bool   `json:"archived"`
			MergedIntoID string `json:"merged_into_id"`
		}
		if err := json.Unmarshal(d.restStatus(t, "GET", "/v1/contacts/"+pair[0]+"?include_archived=true", nil, http.StatusOK), &read); err != nil {
			t.Fatal(err)
		}
		if read.MergedIntoID != pair[1] {
			t.Errorf("contact %s is merged into %q, want %s", pair[0], read.MergedIntoID, pair[1])
		}
	}
	d.sameEffect(t, "merge", restSource, toolSource)
	d.sameEffect(t, "merge survivor", restSurvivor, toolSurvivor)
}
