// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The record writes an agent makes: create_record, update_record,
// archive_record, merge_records, log_activity and create_task. Each test asks
// the route as a passport and the tool over MCP for the same change on two
// equal records, then compares the stored records and what each door left in
// the audit and outbox rows.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// sharedContact is the part of a stored contact that two contacts made the
// same way hold alike.
type sharedContact struct {
	Title      *string `json:"title"`
	Visibility string  `json:"visibility"`
	Archived   bool    `json:"archived"`
}

func (d *twoDoors) storedContact(t *testing.T, id string) json.RawMessage {
	t.Helper()
	return d.restStatus(t, "GET", "/v1/contacts/"+id, nil, http.StatusOK)
}

func versionOf(t *testing.T, raw json.RawMessage) int {
	t.Helper()
	var read struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &read); err != nil || read.Version == 0 {
		t.Fatalf("the record carries no version (%v): %s", err, raw)
	}
	return read.Version
}

// anIDIn reads the id a tool or route answer names, whatever the member is
// called.
func anIDIn(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var named struct {
		ID       string `json:"id"`
		RecordID string `json:"record_id"`
		Result   struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &named); err != nil {
		t.Fatalf("the answer does not decode: %v\n%s", err, raw)
	}
	for _, id := range []string{named.ID, named.RecordID, named.Result.ID} {
		if id != "" {
			return id
		}
	}
	t.Fatalf("the answer names no record: %s", raw)
	return ""
}

// A contact made, edited and archived over either door is left alike: the same
// stored fields, and the same audit and outbox rows naming the agent.
func TestContactWritesLeaveTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "record-write-doors", "read", "write")

	restMade := d.restStatus(t, "POST", "/v1/contacts", AnyMap{"full_name": "Rest Rowan", "title": "Buyer", "source": "manual"}, http.StatusCreated)
	toolMade := d.tool(t, "create_record", map[string]any{
		"record_type": "contact", "fields": map[string]any{"full_name": "Tool Tamsin", "title": "Buyer", "source": "manual"},
	})
	viaREST, viaTool := anIDIn(t, restMade), anIDIn(t, toolMade)
	sameRecords[sharedContact](t, "create", d.storedContact(t, viaREST), d.storedContact(t, viaTool))
	left := d.sameEffect(t, "create", viaREST, viaTool)
	if !strings.HasPrefix(left.Audit[0], "create/agent/") {
		t.Errorf("the creation left %v, want a create row naming the agent", left.Audit)
	}

	version := versionOf(t, d.storedContact(t, viaREST))
	d.restPinned(t, "PATCH", "/v1/contacts/"+viaREST, AnyMap{"title": "Head of Buying"}, version, http.StatusOK)
	d.tool(t, "update_record", map[string]any{
		"record_type": "contact", "id": viaTool, "fields": map[string]any{"title": "Head of Buying"},
		"if_version": versionOf(t, d.storedContact(t, viaTool)),
	})
	edited := sameRecords[sharedContact](t, "update", d.storedContact(t, viaREST), d.storedContact(t, viaTool))
	if edited.Title == nil || *edited.Title != "Head of Buying" {
		t.Errorf("the edited contact carries title %v, want the new one", edited.Title)
	}
	d.sameEffect(t, "update", viaREST, viaTool)

	d.restPinned(t, "DELETE", "/v1/contacts/"+viaREST, nil, versionOf(t, d.storedContact(t, viaREST)), http.StatusOK)
	d.tool(t, "archive_record", map[string]any{"record_type": "contact", "id": viaTool})
	d.sameEffect(t, "archive", viaREST, viaTool)
}

type sharedActivity struct {
	Kind string  `json:"kind"`
	Body *string `json:"body"`
}

// A note logged and a task filed over either door are the same records: the
// same kind and text, linked to the same kind of record, with the same rows
// behind them.
func TestLogActivityAndCreateTaskLeaveTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "activity-write-doors", "read", "write")
	contact := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Linked Lena", "source": "manual"})
	link := []map[string]any{{"entity_type": "contact", "entity_id": contact}}

	restNote := d.restStatus(t, "POST", "/v1/activities", AnyMap{
		"kind": "note", "subject": "Call recap", "body": "Wants a quote by Friday", "source": "manual", "links": link,
	}, http.StatusCreated)
	toolNote := d.tool(t, "log_activity", map[string]any{
		"kind": "note", "subject": "Call recap", "body": "Wants a quote by Friday", "links": link,
	})
	noteREST, noteTool := anIDIn(t, restNote), anIDIn(t, toolNote)
	stored := func(id string) json.RawMessage {
		return d.restStatus(t, "GET", "/v1/activities/"+id, nil, http.StatusOK)
	}
	logged := sameRecords[sharedActivity](t, "note", stored(noteREST), stored(noteTool))
	if logged.Kind != "note" || logged.Body == nil || *logged.Body != "Wants a quote by Friday" {
		t.Errorf("the logged note is %+v, want the note with its text", logged)
	}
	d.sameEffect(t, "note", noteREST, noteTool)

	restTask := d.restStatus(t, "POST", "/v1/tasks", AnyMap{
		"subject": "Send the quote", "due_at": "2036-04-01T09:00:00Z", "source": "manual", "links": link,
	}, http.StatusCreated)
	toolTask := d.tool(t, "create_task", map[string]any{
		"subject": "Send the quote", "due_at": "2036-04-01T09:00:00Z", "links": link,
	})
	taskREST, taskTool := anIDIn(t, restTask), anIDIn(t, toolTask)
	filed := sameRecords[sharedActivity](t, "task", stored(taskREST), stored(taskTool))
	if filed.Kind != "task" {
		t.Errorf("the filed task is a %q, want a task", filed.Kind)
	}
	d.sameEffect(t, "task", taskREST, taskTool)
}
