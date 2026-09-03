// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import "testing"

// Microsoft names an attendee inside emailAddress, where Google uses
// displayName. Both are the same fact and both must survive the decode, or an
// Outlook workspace keeps the local-part naming a Google one no longer has.
func TestAnAttendeeKeepsTheNameTheInviteGave(t *testing.T) {
	raw := []byte(`{
	  "id": "evt-1",
	  "subject": "Pricing review",
	  "start": {"dateTime": "2026-09-03T07:15:00", "timeZone": "UTC"},
	  "organizer": {"emailAddress": {"address": "lars@gradion.com", "name": "Lars Jankowfsky"}},
	  "attendees": [
	    {"type": "required", "emailAddress": {"address": "chris@erlerventures.org", "name": "Chris Erler"}}
	  ]
	}`)
	parties, err := ParticipantsOf(raw, "")
	if err != nil {
		t.Fatalf("ParticipantsOf: %v", err)
	}
	names := map[string]string{}
	for _, p := range parties {
		names[p.Email] = p.DisplayName
	}
	if got := names["chris@erlerventures.org"]; got != "Chris Erler" {
		t.Errorf("attendee name = %q, want %q", got, "Chris Erler")
	}
	if got := names["lars@gradion.com"]; got != "Lars Jankowfsky" {
		t.Errorf("organizer name = %q, want %q", got, "Lars Jankowfsky")
	}
}
