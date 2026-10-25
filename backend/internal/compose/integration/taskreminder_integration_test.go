// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// The reminder filter answers alike through the route and the agent's tool,
// and moving the task's due date over HTTP ends the reminder.
func TestTheReminderFilterAnswersAlikeThroughBothDoors(t *testing.T) {
	d := newTwoDoors(t, "task-reminder", "read", "write")
	now := clocktest.Now(t).UTC()
	var created struct {
		ID string `json:"id"`
	}
	if status := d.e.Call(t, http.MethodPost, "/v1/activities", AnyMap{
		"kind": "task", "subject": "Call back about the renewal",
		"due_at":    now.AddDate(0, 0, 7).Format(time.RFC3339),
		"remind_at": now.Add(-time.Minute).Format(time.RFC3339),
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("writing the task → %d", status)
	}
	if status := d.e.Call(t, http.MethodPost, "/v1/activities", AnyMap{
		"kind": "task", "subject": "Send the quote", "due_at": now.AddDate(0, 0, 7).Format(time.RFC3339),
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("writing the task with no reminder → %d", status)
	}

	sameDoorRecords(t, "the due reminders",
		d.rest(t, http.MethodGet, "/v1/activities?reminder_due=true", nil),
		d.tool(t, "list_records", map[string]any{"record_type": "activity", "filters": map[string]any{"reminder_due": "true"}}),
		created.ID)
	if listed := dueReminderIDs(t, d); len(listed) != 1 || listed[0] != created.ID {
		t.Errorf("the filter lists %v, want only the due reminder", listed)
	}

	var moved struct {
		RemindAt *string `json:"remind_at"`
	}
	if status := d.e.Call(t, http.MethodPatch, "/v1/activities/"+created.ID,
		AnyMap{"due_at": now.AddDate(0, 0, 8).Format(time.RFC3339)}, nil, &moved); status != http.StatusOK {
		t.Fatalf("moving the task → %d", status)
	}
	if moved.RemindAt != nil {
		t.Errorf("moving the due date left the passed reminder at %s", *moved.RemindAt)
	}
	if listed := dueReminderIDs(t, d); len(listed) != 0 {
		t.Errorf("after the move the filter lists %v, want none", listed)
	}

	var undated struct {
		ID string `json:"id"`
	}
	if status := d.e.Call(t, http.MethodPost, "/v1/activities", AnyMap{
		"kind": "task", "subject": "Ring back some day",
		"due_at":    now.AddDate(0, 0, 7).Format(time.RFC3339),
		"remind_at": now.Add(-time.Minute).Format(time.RFC3339),
	}, nil, &undated); status != http.StatusCreated {
		t.Fatalf("writing the second task → %d", status)
	}
	var cleared struct {
		RemindAt *string `json:"remind_at"`
	}
	if status := d.e.Call(t, http.MethodPatch, "/v1/activities/"+undated.ID,
		AnyMap{"due_at": nil}, nil, &cleared); status != http.StatusOK {
		t.Fatalf("clearing the due date → %d", status)
	}
	if cleared.RemindAt != nil {
		t.Errorf("clearing the due date left the passed reminder at %s", *cleared.RemindAt)
	}
}

func dueReminderIDs(t *testing.T, d *twoDoors) []string {
	t.Helper()
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := d.e.Call(t, http.MethodGet, "/v1/activities?reminder_due=true", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("listing the due reminders → %d", status)
	}
	out := make([]string, 0, len(page.Data))
	for _, row := range page.Data {
		out = append(out, row.ID)
	}
	return out
}
