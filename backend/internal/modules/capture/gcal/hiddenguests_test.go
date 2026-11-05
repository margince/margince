// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"encoding/json"
	"maps"
	"testing"
)

// A copy of the event that Google may have cut short, or whose guest list is
// hidden from guests, cannot prove the meeting internal. The organizer's own
// copy of a hidden list shows everyone, so it still can.
func TestACopyWithHiddenGuestsClaimsNoOwnerDomain(t *testing.T) {
	cases := []struct {
		name       string
		fields     map[string]any
		incomplete bool
	}{
		{"a full list", map[string]any{}, false},
		{"attendees omitted", map[string]any{"attendeesOmitted": true}, true},
		{"guest list hidden from a guest", map[string]any{"guestsCanSeeOtherGuests": false}, true},
		{"guest list shown", map[string]any{"guestsCanSeeOtherGuests": true}, false},
		{"guest list hidden, read by its organizer", map[string]any{
			"guestsCanSeeOtherGuests": false,
			"organizer":               map[string]any{"email": "peer@myco.com", "self": true},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := map[string]any{
				"id": "evt-hidden", "status": "confirmed", "summary": "Kick-off",
				"start":     map[string]string{"dateTime": "2026-07-16T09:00:00Z"},
				"organizer": map[string]any{"email": "peer@myco.com"},
				"attendees": []map[string]any{{"email": gcalOwner, "self": true, "responseStatus": "accepted"}},
			}
			maps.Copy(event, tc.fields)
			raw, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			rec := mustParse(t, raw).ToRecord(connectorName, raw)
			if rec.PartiesIncomplete != tc.incomplete {
				t.Errorf("PartiesIncomplete = %v, want %v", rec.PartiesIncomplete, tc.incomplete)
			}
			if wantDomain := map[bool]string{false: "myco.com", true: ""}[tc.incomplete]; rec.OwnerDomainOnly != wantDomain {
				t.Errorf("OwnerDomainOnly = %q, want %q", rec.OwnerDomainOnly, wantDomain)
			}
		})
	}
}
