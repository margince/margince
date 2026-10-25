// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import "testing"

// An attendee's copy of a meeting with hidden attendees lists only themselves,
// so it cannot prove the meeting internal. The organizer's copy lists everyone.
func TestAnAttendeesCopyWithHiddenAttendeesIsIncomplete(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		hide, organizer, partial bool
	}{
		{"attendees shown", false, false, false},
		{"hidden from an attendee", true, false, true},
		{"hidden, read by the organizer", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := eventJSONWith(t, map[string]any{
				"id": "evt-hidden", "subject": "Kick-off",
				"start":         map[string]string{"dateTime": "2026-07-16T09:00:00.0000000", "timeZone": "UTC"},
				"organizer":     actorJSON("peer@myco.com"),
				"attendees":     attendeesJSON(owner),
				"hideAttendees": tc.hide,
				"isOrganizer":   tc.organizer,
			})
			if got := mustParse(t, raw).ToRecord(connectorName, raw).PartiesIncomplete; got != tc.partial {
				t.Errorf("PartiesIncomplete = %v, want %v", got, tc.partial)
			}
		})
	}
}
