// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
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

// The type an agent picks reaches the activity writer, which decides the
// task's one button from it.
func TestCreateTaskCarriesItsType(t *testing.T) {
	out, err := TaskAsActivity(json.RawMessage(`{"subject":"Call Carol back","task_type":"reach_out"}`))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatal(err)
	}
	if body["task_type"] != "reach_out" {
		t.Fatalf("folded body %v, want task_type reach_out", body)
	}
}

// An agent's task says why it exists, or it is refused before anything is
// written.
func TestCreateTaskRefusesATaskWithNoReason(t *testing.T) {
	for _, args := range []string{`{"subject":"Call back"}`, `{"subject":"Call back","body":"  "}`} {
		var bad *BadArgsError
		if err := requireTaskReason(json.RawMessage(args)); !errors.As(err, &bad) {
			t.Fatalf("%s: err = %v, want a bad-arguments refusal", args, err)
		}
	}
	if err := requireTaskReason(json.RawMessage(`{"subject":"Call back","body":"Anna Weber asked for a call on Tuesday."}`)); err != nil {
		t.Fatalf("a task with its reason was refused: %v", err)
	}
}
