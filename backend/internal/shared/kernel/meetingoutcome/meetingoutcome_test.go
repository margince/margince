// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingoutcome

import "testing"

// Only a recorded held outcome is held. A meeting with no status is awaiting
// its outcome, and booked, canceled and no_show are not held either.
func TestOnlyARecordedHeldOutcomeIsHeld(t *testing.T) {
	cases := map[string]bool{"held": true, "": false, "booked": false, "canceled": false, "no_show": false}
	for status, want := range cases {
		if got := IsHeld(status); got != want {
			t.Errorf("IsHeld(%q) = %t, want %t", status, got, want)
		}
	}
}

// The SQL form names the same value as the Go form, and admits no NULL.
func TestTheSQLFormAdmitsOnlyTheHeldValue(t *testing.T) {
	if got, want := HeldSQL("m"), "(m.meeting_status = 'held')"; got != want {
		t.Errorf("HeldSQL = %q, want %q", got, want)
	}
}
