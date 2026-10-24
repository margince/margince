// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The five tag writes: create_tag, update_tag, apply_tag, remove_tag and
// merge_tags. Each test asks both doors for the same change on two equal
// records. It compares what the two answered or stored, and the audit and
// outbox rows each left.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type sharedTag struct {
	Color       *string `json:"color"`
	Description *string `json:"description"`
	Suggestible bool    `json:"suggestible"`
}

// taggedName reads the tag an answer names. The route calls its id `id` and the
// tool `tag_id`; the word and its id are the same record either way.
type taggedName struct {
	ID    string `json:"id"`
	TagID string `json:"tag_id"`
	Name  string `json:"name"`
}

func idOf(t *testing.T, raw json.RawMessage) taggedName {
	t.Helper()
	var named taggedName
	if err := json.Unmarshal(raw, &named); err != nil {
		t.Fatalf("the answer does not decode: %v\n%s", err, raw)
	}
	if named.ID == "" {
		named.ID = named.TagID
	}
	if named.ID == "" {
		t.Fatalf("the answer names no tag: %s", raw)
	}
	return named
}

func (d *twoDoors) storedTag(t *testing.T, tag string) json.RawMessage {
	t.Helper()
	return d.restStatus(t, "GET", "/v1/tags/"+tag, nil, http.StatusOK)
}

// tagsOn answers the tag ids a contact carries, read through the tool.
func (d *twoDoors) tagsOn(t *testing.T, contact string) []string {
	t.Helper()
	var carried struct {
		Tags []struct {
			TagID string `json:"tag_id"`
		} `json:"tags"`
	}
	raw := d.toolSeed(t, "get_record_tags", map[string]any{"record_type": "contact", "record_id": contact})
	if err := json.Unmarshal(raw, &carried); err != nil {
		t.Fatalf("the tags of %s do not decode: %v\n%s", contact, err, raw)
	}
	var tags []string
	for _, tag := range carried.Tags {
		tags = append(tags, tag.TagID)
	}
	return tags
}

// A tag coined over either door is the same word with the same colour, and
// each coining writes one create row naming the agent. The tool answers a tag
// without its text, so an edit is compared on the stored tag.
func TestCreateAndUpdateTagLeaveTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "tag-write-doors", "read", "write")

	rest := d.restStatus(t, "POST", "/v1/tags", AnyMap{"name": "Rest conference", "color": "teal", "description": "Met at the spring show"}, http.StatusCreated)
	tool := d.tool(t, "create_tag", map[string]any{"name": "Tool conference", "color": "teal", "description": "Met at the spring show"})
	// The tool answers the word without its text, so the answers are compared on
	// the colour and the stored tags below on everything.
	made := sameRecords[struct {
		Color *string `json:"color"`
	}](t, "create", rest, tool)
	if made.Color == nil || *made.Color != "teal" {
		t.Errorf("the coined tag carries colour %v, want teal", made.Color)
	}
	viaREST, viaTool := idOf(t, rest), idOf(t, tool)
	if viaREST.Name != "Rest conference" || viaTool.Name != "Tool conference" {
		t.Errorf("the answers name %q and %q", viaREST.Name, viaTool.Name)
	}
	sameStored(t, "create", d.storedTag(t, viaREST.ID), d.storedTag(t, viaTool.ID), viaREST.ID, viaTool.ID, "name")
	if stored := sameRecords[sharedTag](t, "create", d.storedTag(t, viaREST.ID), d.storedTag(t, viaTool.ID)); stored.Description == nil || *stored.Description != "Met at the spring show" {
		t.Errorf("the coined tag stores description %v, want the text both doors were given", stored.Description)
	}
	left := d.sameEffect(t, "create", viaREST.ID, viaTool.ID, "name")
	if !strings.HasPrefix(left.Audit[0], "create | agent | ") {
		t.Errorf("the coining left %v, want a create row naming the agent", left.Audit)
	}

	edit := map[string]any{"color": "rose", "description": "Spring push"}
	d.restStatus(t, "PATCH", "/v1/tags/"+viaREST.ID, edit, http.StatusOK)
	d.tool(t, "update_tag", with(edit, "tag_id", viaTool.ID))
	sameStored(t, "update", d.storedTag(t, viaREST.ID), d.storedTag(t, viaTool.ID), viaREST.ID, viaTool.ID, "name")
	edited := sameRecords[sharedTag](t, "update", d.storedTag(t, viaREST.ID), d.storedTag(t, viaTool.ID))
	if edited.Color == nil || *edited.Color != "rose" || edited.Description == nil || *edited.Description != "Spring push" {
		t.Errorf("the edited tag carries colour %v and text %v, want rose and the new text", edited.Color, edited.Description)
	}
	d.sameEffect(t, "update", viaREST.ID, viaTool.ID, "name")

	blank := map[string]any{"color": "none", "description": ""}
	d.restStatus(t, "PATCH", "/v1/tags/"+viaREST.ID, blank, http.StatusOK)
	d.tool(t, "update_tag", with(blank, "tag_id", viaTool.ID))
	cleared := sameRecords[sharedTag](t, "clear", d.storedTag(t, viaREST.ID), d.storedTag(t, viaTool.ID))
	if cleared.Color != nil {
		t.Errorf("colour none left the colour at %q", *cleared.Color)
	}

	d.restStatus(t, "DELETE", "/v1/tags/"+viaREST.ID, nil, http.StatusOK)
	d.tool(t, "archive_tag", map[string]any{"tag_id": viaTool.ID})
	sameStored(t, "archive", d.storedTag(t, viaREST.ID), d.storedTag(t, viaTool.ID), viaREST.ID, viaTool.ID, "name")
	d.sameEffect(t, "archive", viaREST.ID, viaTool.ID, "name")
}

// A tag put on a contact and taken off again leaves the same rows on either
// door. The route answers a handle that puts the tagging back, and the tool
// answers the state. So the two are compared on what each left behind. The rows
// sit on the tag, so each door has its own.
func TestApplyAndRemoveTagLeaveTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "tagging-write-doors", "read", "write")
	tagREST := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Key account rest", "color": "amber"})
	tagTool := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Key account tool", "color": "amber"})
	viaREST := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Rest Tagged", "source": "manual"})
	viaTool := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Tool Tagged", "source": "manual"})

	d.restStatus(t, "POST", "/v1/tags/"+tagREST+"/apply", AnyMap{"entity_type": "contact", "entity_id": viaREST}, http.StatusCreated)
	d.tool(t, "apply_tag", map[string]any{"tag_id": tagTool, "record_type": "contact", "record_id": viaTool})
	if carried := d.tagsOn(t, viaREST); len(carried) != 1 || carried[0] != tagREST {
		t.Errorf("contact %s carries %v, want the one tag applied", viaREST, carried)
	}
	if carried := d.tagsOn(t, viaTool); len(carried) != 1 || carried[0] != tagTool {
		t.Errorf("contact %s carries %v, want the one tag applied", viaTool, carried)
	}
	d.sameEffect(t, "apply", tagREST, tagTool, "name")

	var handle struct {
		AuditID string `json:"audit_id"`
	}
	restOff := d.restStatus(t, "DELETE", "/v1/tags/"+tagREST+"/apply", AnyMap{"entity_type": "contact", "entity_id": viaREST}, http.StatusOK)
	if err := json.Unmarshal(restOff, &handle); err != nil || handle.AuditID == "" {
		t.Errorf("the route answered no undo handle (%v): %s", err, restOff)
	}
	d.tool(t, "remove_tag", map[string]any{"tag_id": tagTool, "record_type": "contact", "record_id": viaTool})
	for _, contact := range []string{viaREST, viaTool} {
		if carried := d.tagsOn(t, contact); len(carried) != 0 {
			t.Errorf("contact %s still carries %v after the removal", contact, carried)
		}
	}
	d.sameEffect(t, "remove", tagREST, tagTool, "name")

	// A tag the workspace does not hold is a 404 over REST and already-removed
	// over the tool, which asks for the state and finds it true. That is accepted.
	var absent struct {
		Applied bool `json:"applied"`
	}
	unknown := d.tool(t, "remove_tag", map[string]any{"tag_name": "No such tag", "record_type": "contact", "record_id": viaTool})
	if err := json.Unmarshal(unknown, &absent); err != nil || absent.Applied {
		t.Errorf("the tool answered %s for a tag name the workspace lacks, want applied false (%v)", unknown, err)
	}
	d.restStatus(t, "DELETE", "/v1/tags/"+viaTool+"/apply", AnyMap{"entity_type": "contact", "entity_id": viaTool}, http.StatusNotFound)
}

// Folding one tag into another runs at once on a colleague's passport, as the
// colleague could in the app. Both doors leave the same rows and stage nothing.
func TestMergeTagsOnAColleaguesPassportRunsTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "tag-merge-doors", "read", "write")
	into := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Conference 2026", "color": "sky"})
	viaREST := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Conf 26 rest", "color": "sky"})
	viaTool := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Conf 26 tool", "color": "sky"})

	var merged json.RawMessage
	if status := d.e.Call(t, "POST", "/v1/tags/"+viaREST+"/merge", AnyMap{"into_tag_id": into}, d.bearer, &merged); status != http.StatusOK {
		t.Fatalf("a merge over REST → %d %s, want it run", status, merged)
	}
	d.tool(t, "merge_tags", map[string]any{"tag_id": viaTool, "into_tag_id": into})
	d.sameEffect(t, "merge", viaREST, viaTool, "name", "summary")
	var staged int
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM approval`).Scan(&staged); err != nil {
		t.Fatalf("counting approvals: %v", err)
	}
	if staged != 0 {
		t.Errorf("the two merges staged %d approvals, want none", staged)
	}
}
