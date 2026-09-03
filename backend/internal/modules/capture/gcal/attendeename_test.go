// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import "testing"

// The name an invitation gives an attendee is the only full name a contact
// minted from a bare address ever gets. Google sends it as displayName, and
// this connector dropped it at decode — so a person invited as
// `chris@example.org` was filed as "Chris" and stayed that way.
func TestAnAttendeeKeepsTheNameTheInviteGave(t *testing.T) {
	raw := []byte(`{
	  "id": "evt-1",
	  "status": "confirmed",
	  "summary": "Chris Erler and Lars Jankowfsky",
	  "start": {"dateTime": "2026-09-03T07:15:00Z"},
	  "organizer": {"email": "lars@gradion.com", "displayName": "Lars Jankowfsky"},
	  "attendees": [
	    {"email": "lars@gradion.com", "organizer": true},
	    {"email": "chris@erlerventures.org", "displayName": "Chris Erler"}
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
		t.Errorf("organizer name = %q, want %q — the organizer is named too", got, "Lars Jankowfsky")
	}
}

// An invitation that names nobody must say so with an empty name rather than
// inventing one: ” is what the stamp records when the provider was silent.
func TestAnUnnamedAttendeeCarriesNoName(t *testing.T) {
	raw := []byte(`{
	  "id": "evt-2",
	  "status": "confirmed",
	  "start": {"dateTime": "2026-09-03T07:15:00Z"},
	  "organizer": {"email": "lars@gradion.com"},
	  "attendees": [{"email": "chris@erlerventures.org"}]
	}`)
	parties, err := ParticipantsOf(raw, "")
	if err != nil {
		t.Fatalf("ParticipantsOf: %v", err)
	}
	for _, p := range parties {
		if p.DisplayName != "" {
			t.Errorf("%s carries the name %q, which the invitation never gave", p.Email, p.DisplayName)
		}
	}
}
