// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import "testing"

func TestAStoredEventProvesItsCalendarOnlyWhenOneConnectedAddressFits(t *testing.T) {
	connected := []string{"Rep@myco.com", "peer@myco.com"}
	for _, tc := range []struct {
		name      string
		organizer string
		isOwn     bool
		guests    []string
		want      string
	}{
		{"the organizer's own copy", "Host@myco.com", true, []string{"buyer@customer.com"}, "host@myco.com"},
		{"a guest's copy listing one connected address", "buyer@customer.com", false, []string{"rep@myco.com", "other@customer.com"}, "rep@myco.com"},
		{"one connected address listed twice", "buyer@customer.com", false, []string{"rep@myco.com", "REP@myco.com"}, "rep@myco.com"},
		{"a guest's copy listing two connected addresses", "buyer@customer.com", false, []string{"rep@myco.com", "peer@myco.com"}, ""},
		{"a guest's copy listing no connected address", "buyer@customer.com", false, []string{"other@customer.com"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := eventJSONWith(t, map[string]any{
				"id": "evt-own", "organizer": actorJSON(tc.organizer),
				"attendees": attendeesJSON(tc.guests...), "isOrganizer": tc.isOwn,
			})
			got, err := OwnCalendar(raw, connected)
			if err != nil || got != tc.want {
				t.Errorf("OwnCalendar = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	if _, err := OwnCalendar([]byte("{"), connected); err == nil {
		t.Error("an unreadable event proved a calendar")
	}
}
