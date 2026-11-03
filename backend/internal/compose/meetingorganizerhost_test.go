// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"
)

// Handing a synced meeting to its organizer is a calendar fact, not an edit.
// The history offers no undo, because a replay cannot write a meeting's host.
func TestTheOrganizerHostMoveOffersNoUndo(t *testing.T) {
	row := AuditRow{
		EntityType: entityTypeActivity, Action: "update",
		Before:   []byte(`{"host_user_id":"0195e4a0-0000-7000-8000-000000000002"}`),
		After:    []byte(`{"host_user_id":"0195e4a0-0000-7000-8000-000000000001"}`),
		Evidence: []byte(`{"reason":"meeting_organizer_host"}`),
	}
	answer, err := Evaluator{}.Evaluate(context.Background(), nil, row, Advisory)
	if err != nil {
		t.Fatalf("judging the host move: %v", err)
	}
	if answer.Undoable || answer.Reason != ReasonNotRestorableByThisPath {
		t.Errorf("the host move reads %+v, want refused as not restorable by this path", answer)
	}
}
