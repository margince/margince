// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"testing"
)

// An agent that agrees a callback hour says so, and the activity writer then
// sets the reminder to it. The fold must carry the flag, or the hour reads as
// the end of a day.
func TestCreateTaskCarriesTheTimeOfDay(t *testing.T) {
	out, err := TaskAsActivity(json.RawMessage(`{"subject":"Call back","due_at":"2027-03-02T14:00:00Z","due_has_time":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatal(err)
	}
	if body["due_has_time"] != true || body["due_at"] != "2027-03-02T14:00:00Z" || body["kind"] != "task" {
		t.Fatalf("folded body %v, want the task with its time of day", body)
	}
	day, err := TaskAsActivity(json.RawMessage(`{"subject":"Send the quote","due_at":"2027-03-02T22:59:59Z"}`))
	if err != nil {
		t.Fatal(err)
	}
	var dayBody map[string]any
	if err := json.Unmarshal(day, &dayBody); err != nil {
		t.Fatal(err)
	}
	if _, named := dayBody["due_has_time"]; named {
		t.Fatalf("a task given a day alone carries a time flag: %v", dayBody)
	}
}
