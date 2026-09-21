// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

// Reading the connected calendar's own RSVP off a Graph event.
//
// Graph states the owner's answer at the top level of the event, so this
// connector needs no search through the attendee list the way Google's does —
// but the RULE is the same one, and it is proved here so the two calendars
// cannot drift onto different answers about what a decline means.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
)

const rsvpOwner = "rep@myco.com"

// rsvpEventJSON builds a live event whose owner has answered `response`.
func rsvpEventJSON(t *testing.T, response string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"id":             "evt-rsvp",
		"subject":        "Consulting Monthly",
		"isCancelled":    false,
		"start":          map[string]string{"dateTime": "2026-03-04T09:00:00.0000000", "timeZone": "UTC"},
		"organizer":      map[string]any{"emailAddress": map[string]string{"address": "client@acme.test"}},
		"responseStatus": map[string]string{"response": response},
		"attendees": []map[string]any{
			{"type": "required", "emailAddress": map[string]string{"address": rsvpOwner}},
			{"type": "required", "emailAddress": map[string]string{"address": "client@acme.test"}},
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return raw
}

func TestTheOwnersDeclineIsReadOffTheResponseStatus(t *testing.T) {
	t.Parallel()
	// Only a refusal takes a meeting off the schedule. Tentative is an
	// attendance somebody may yet make, and notResponded is an invitation
	// nobody has opened.
	for response, want := range map[string]bool{
		"declined":            true,
		"accepted":            false,
		"tentativelyAccepted": false,
		"notResponded":        false,
		"organizer":           false,
		"none":                false,
	} {
		t.Run(response, func(t *testing.T) {
			t.Parallel()
			ev, err := decodeEvent(rsvpEventJSON(t, response), rsvpOwner)
			if err != nil {
				t.Fatalf("decodeEvent: %v", err)
			}
			if ev.OwnerDeclined != want {
				t.Fatalf("OwnerDeclined = %v for response %q, want %v", ev.OwnerDeclined, response, want)
			}
		})
	}
}

// End to end through this vendor's decode: a declined meeting settles as a
// cancellation, so the row already captured gets closed.
func TestADeclinedInvitationSettlesAsACancellation(t *testing.T) {
	t.Parallel()
	ev, err := decodeEvent(rsvpEventJSON(t, "declined"), rsvpOwner)
	if err != nil {
		t.Fatalf("decodeEvent: %v", err)
	}
	if _, got := meetingmap.Classify(ev, rsvpOwner).Settle(); got != meetingmap.SettleCancel {
		t.Fatalf("Settle() = %v, want the declined meeting closed", got)
	}
}
