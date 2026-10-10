// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The five tag writes: create_tag, update_tag, apply_tag, remove_tag and
// merge_tags. Each test asks the route as a passport and the tool over MCP for
// the same change on two equal records, then compares what the two answered
// or stored and what each left in the audit and outbox rows.

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
	raw := d.tool(t, "get_record_tags", map[string]any{"record_type": "contact", "record_id": contact})
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

	rest := d.restStatus(t, "POST", "/v1/tags", AnyMap{"name": "Rest conference", "color": "teal"}, http.StatusCreated)
	tool := d.tool(t, "create_tag", map[string]any{"name": "Tool conference", "color": "teal"})
	made := sameRecords[sharedTag](t, "create", rest, tool)
	if made.Color == nil || *made.Color != "teal" {
		t.Errorf("the coined tag carries colour %v, want teal", made.Color)
	}
	viaREST, viaTool := idOf(t, rest), idOf(t, tool)
	if viaREST.Name != "Rest conference" || viaTool.Name != "Tool conference" {
		t.Errorf("the answers name %q and %q", viaREST.Name, viaTool.Name)
	}
	left := d.sameEffect(t, "create", viaREST.ID, viaTool.ID)
	if !strings.HasPrefix(left.Audit[0], "create/agent/") {
		t.Errorf("the coining left %v, want a create row naming the agent", left.Audit)
	}

	edit := map[string]any{"color": "rose", "description": "Spring push"}
	d.restStatus(t, "PATCH", "/v1/tags/"+viaREST.ID, edit, http.StatusOK)
	d.tool(t, "update_tag", with(edit, "tag_id", viaTool.ID))
	edited := sameRecords[sharedTag](t, "update", d.storedTag(t, viaREST.ID), d.storedTag(t, viaTool.ID))
	if edited.Color == nil || *edited.Color != "rose" || edited.Description == nil || *edited.Description != "Spring push" {
		t.Errorf("the edited tag carries colour %v and text %v, want rose and the new text", edited.Color, edited.Description)
	}
	d.sameEffect(t, "update", viaREST.ID, viaTool.ID)

	blank := map[string]any{"color": "none", "description": ""}
	d.restStatus(t, "PATCH", "/v1/tags/"+viaREST.ID, blank, http.StatusOK)
	d.tool(t, "update_tag", with(blank, "tag_id", viaTool.ID))
	cleared := sameRecords[sharedTag](t, "clear", d.storedTag(t, viaREST.ID), d.storedTag(t, viaTool.ID))
	if cleared.Color != nil {
		t.Errorf("colour none left the colour at %q", *cleared.Color)
	}
}

// A tag put on a contact and taken off again leaves the same rows on either
// door. The route answers a handle that puts the tagging back and the tool
// answers the state, so the two are compared on what each left behind.
func TestApplyAndRemoveTagLeaveTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "tagging-write-doors", "read", "write")
	tag := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Key account", "color": "amber"})
	viaREST := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Rest Tagged", "source": "manual"})
	viaTool := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Tool Tagged", "source": "manual"})

	d.restStatus(t, "POST", "/v1/tags/"+tag+"/apply", AnyMap{"entity_type": "contact", "entity_id": viaREST}, http.StatusCreated)
	d.tool(t, "apply_tag", map[string]any{"tag_id": tag, "record_type": "contact", "record_id": viaTool})
	for _, contact := range []string{viaREST, viaTool} {
		if carried := d.tagsOn(t, contact); len(carried) != 1 || carried[0] != tag {
			t.Errorf("contact %s carries %v, want the one tag applied", contact, carried)
		}
	}
	d.sameEffect(t, "apply", viaREST, viaTool)

	var handle struct {
		AuditID string `json:"audit_id"`
	}
	restOff := d.restStatus(t, "DELETE", "/v1/tags/"+tag+"/apply", AnyMap{"entity_type": "contact", "entity_id": viaREST}, http.StatusOK)
	if err := json.Unmarshal(restOff, &handle); err != nil || handle.AuditID == "" {
		t.Errorf("the route answered no undo handle (%v): %s", err, restOff)
	}
	d.tool(t, "remove_tag", map[string]any{"tag_id": tag, "record_type": "contact", "record_id": viaTool})
	for _, contact := range []string{viaREST, viaTool} {
		if carried := d.tagsOn(t, contact); len(carried) != 0 {
			t.Errorf("contact %s still carries %v after the removal", contact, carried)
		}
	}
	d.sameEffect(t, "remove", viaREST, viaTool)
}

// Folding one tag into another is confirm-first for an agent: each door stages
// one approval of the same kind and runs nothing until a human approves.
func TestMergeTagsStagesTheSameApprovalOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "tag-merge-doors", "read", "write")
	into := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Conference 2026", "color": "sky"})
	viaREST := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Conf 26 rest", "color": "sky"})
	viaTool := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Conf 26 tool", "color": "sky"})

	var refusal fileRefusal
	if status := d.e.Call(t, "POST", "/v1/tags/"+viaREST+"/merge", AnyMap{"into_tag_id": into}, d.bearer, &refusal); status != http.StatusForbidden || refusal.Code != "approval_required" {
		t.Fatalf("a merge over REST → %d %q, want 403 approval_required", status, refusal.Code)
	}
	d.toolRefused(t, "merge_tags", map[string]any{"tag_id": viaTool, "into_tag_id": into})
	card := d.sameStagedKind(t, "merge", viaREST, viaTool)
	if card.Kind != "merge_tags" || card.Status != "pending" {
		t.Errorf("the staged approval is %+v, want a pending merge_tags one", card)
	}
}
