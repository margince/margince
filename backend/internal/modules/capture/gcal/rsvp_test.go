// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

// Reading the connected account's own RSVP off a Calendar event.
//
// A declined invitation is the case a reader actually sees: Google leaves the
// event on the calendar with status "confirmed" and strikes it through, so
// nothing about the event itself says it is off. Only the owner's own attendee
// entry does, and missing it is what kept a declined meeting on the schedule.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
)

// rsvpEventJSON builds a confirmed event whose attendees carry RSVPs. Each
// attendee is {email, responseStatus, self}, which is the shape Calendar sends.
func rsvpEventJSON(t *testing.T, attendees []map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"id":        "evt-rsvp",
		"status":    "confirmed",
		"summary":   "Consulting Monthly",
		"start":     map[string]string{"dateTime": "2026-03-04T09:00:00Z"},
		"organizer": map[string]string{"email": "client@acme.test"},
		"attendees": attendees,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return raw
}

const rsvpOwner = "rep@myco.com"

func TestTheOwnersDeclineIsReadOffTheAttendeeList(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		attendees []map[string]any
		want      bool
	}{
		// Google's own answer to "which of these is you". It holds even when
		// the address does not match, which is the alias case below.
		"declined, marked self": {
			attendees: []map[string]any{
				{"email": rsvpOwner, "responseStatus": "declined", "self": true},
				{"email": "client@acme.test", "responseStatus": "accepted"},
			},
			want: true,
		},
		// The account was invited under an alias it never told us about, so
		// matching the owner's address finds nobody. The flag still answers.
		"declined under an alias": {
			attendees: []map[string]any{
				{"email": "sales@myco.com", "responseStatus": "declined", "self": true},
				{"email": "client@acme.test", "responseStatus": "accepted"},
			},
			want: true,
		},
		// The fallback: a payload carrying no flag is still readable by address.
		"declined, no self flag": {
			attendees: []map[string]any{
				{"email": rsvpOwner, "responseStatus": "declined"},
				{"email": "client@acme.test", "responseStatus": "accepted"},
			},
			want: true,
		},
		// The `self` entry is the AUTHORITY, so its answer settles the question
		// and the address is not consulted afterwards. An event can carry the
		// owner's address twice — a room or delegate entry, a duplicated
		// invitation — and reading the two signals as a pair of equals would let
		// the second entry's decline overrule the account's own acceptance,
		// taking a meeting the rep is going to off their schedule.
		"self accepted, a second entry on the owner's address declined": {
			attendees: []map[string]any{
				{"email": rsvpOwner, "responseStatus": "accepted", "self": true},
				{"email": rsvpOwner, "responseStatus": "declined"},
				{"email": "client@acme.test", "responseStatus": "accepted"},
			},
			want: false,
		},
		// SOMEBODY ELSE declining is not the owner declining. The meeting is
		// still on this rep's calendar and still theirs to prepare.
		"a guest declined": {
			attendees: []map[string]any{
				{"email": rsvpOwner, "responseStatus": "accepted", "self": true},
				{"email": "client@acme.test", "responseStatus": "declined"},
			},
			want: false,
		},
		// Neither of the two answers that are not a refusal takes a meeting off
		// the schedule: tentative is an attendance somebody may yet make, and
		// needsAction is an invitation nobody has opened.
		"tentative": {
			attendees: []map[string]any{
				{"email": rsvpOwner, "responseStatus": "tentative", "self": true},
				{"email": "client@acme.test", "responseStatus": "accepted"},
			},
			want: false,
		},
		"needsAction": {
			attendees: []map[string]any{
				{"email": rsvpOwner, "responseStatus": "needsAction", "self": true},
				{"email": "client@acme.test", "responseStatus": "accepted"},
			},
			want: false,
		},
		"accepted": {
			attendees: []map[string]any{
				{"email": rsvpOwner, "responseStatus": "accepted", "self": true},
				{"email": "client@acme.test", "responseStatus": "accepted"},
			},
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ev, err := decodeEvent(rsvpEventJSON(t, tc.attendees), rsvpOwner)
			if err != nil {
				t.Fatalf("decodeEvent: %v", err)
			}
			if ev.OwnerDeclined != tc.want {
				t.Fatalf("OwnerDeclined = %v, want %v", ev.OwnerDeclined, tc.want)
			}
		})
	}
}

// The whole point, end to end through this vendor's decode: a declined meeting
// settles as a cancellation, so the row already captured gets closed.
func TestADeclinedInvitationSettlesAsACancellation(t *testing.T) {
	t.Parallel()
	raw := rsvpEventJSON(t, []map[string]any{
		{"email": rsvpOwner, "responseStatus": "declined", "self": true},
		{"email": "client@acme.test", "responseStatus": "accepted"},
	})
	ev, err := decodeEvent(raw, rsvpOwner)
	if err != nil {
		t.Fatalf("decodeEvent: %v", err)
	}
	if _, got := meetingmap.Classify(ev, rsvpOwner).Settle(); got != meetingmap.SettleCancel {
		t.Fatalf("Settle() = %v, want the declined meeting closed", got)
	}
}
