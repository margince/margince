// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"
	"testing"
)

func TestAnInvitationCapsAndFoldsItsAttendees(t *testing.T) {
	many := make([]string, maxInvitationAttendees)
	for i := range many {
		many[i] = "guest" + string(rune('a'+i)) + "@example.test"
	}
	if _, err := normalizedAttendees(many, "primary@example.test"); err == nil {
		t.Error("26 attendees were accepted")
	}
	if _, err := normalizedAttendees(many[:24], "primary@example.test"); err != nil {
		t.Errorf("25 attendees were refused: %v", err)
	}
	if _, err := normalizedAttendees([]string{"not an address"}); err == nil {
		t.Error("a malformed address was accepted")
	}
	folded, err := normalizedAttendees([]string{" A@Example.test", "a@example.test", "primary@example.test"}, "Primary@example.test")
	if err != nil || !slices.Equal(folded, []string{"a@example.test"}) {
		t.Errorf("folded = %v %v, want one lower-cased address beside the primary", folded, err)
	}
}
