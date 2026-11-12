// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestOnlyACallersDoneOnADoneTaskIgnoresTheVersion(t *testing.T) {
	done, open := true, false
	subject := "Call back"
	due := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	doneTask := crmcontracts.Activity{Kind: crmcontracts.ActivityKindTask, IsDone: &done}
	openTask := crmcontracts.Activity{Kind: crmcontracts.ActivityKindTask, IsDone: &open}
	callersDone := func(edit func(*UpdateActivityInput)) UpdateActivityInput {
		in := activityUpdateInput(crmcontracts.UpdateActivityRequest{IsDone: &done}, new(int64(3)))
		edit(&in)
		return in
	}
	cases := []struct {
		name    string
		in      UpdateActivityInput
		current crmcontracts.Activity
		want    bool
	}{
		{"Done alone on a done task", callersDone(func(*UpdateActivityInput) {}), doneTask, true},
		{"Done alone on an open task", callersDone(func(*UpdateActivityInput) {}), openTask, false},
		{"reopening a done task", callersDone(func(in *UpdateActivityInput) { in.IsDone = &open }), doneTask, false},
		{"Done with a new subject", callersDone(func(in *UpdateActivityInput) { in.Subject = &subject }), doneTask, false},
		{"Done with a new due date", callersDone(func(in *UpdateActivityInput) { in.DueAt = &due }), doneTask, false},
		{"Done with a cleared field", callersDone(func(in *UpdateActivityInput) { in.Clear = []string{"due_at"} }), doneTask, false},
		{"Done with a custom value", callersDone(func(in *UpdateActivityInput) { in.CustomFields = map[string]any{"cf_tier": "a"} }), doneTask, false},
		{"a machine completion", UpdateActivityInput{IsDone: &done, IfVersion: new(int64(3))}, doneTask, false},
	}
	for _, tc := range cases {
		if got := tc.in.asksOnlyForTheDoneItHolds(tc.current); got != tc.want {
			t.Errorf("%s: ignores the version = %v, want %v", tc.name, got, tc.want)
		}
	}
}
