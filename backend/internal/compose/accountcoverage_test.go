// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
)

func TestADayEntryIsReadByItsDateInAZoneAheadOfUTC(t *testing.T) {
	zone := time.FixedZone("UTC+14", 14*60*60)
	dayAnchor := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 11, 3, 23, 0, 0, 0, zone)

	cases := []struct {
		name    string
		contact company360.LastContact
		want    int
	}{
		{"a day entry dated today", company360.LastContact{At: dayAnchor, Precision: "day"}, 0},
		{"an exchange at a moment, read by its local date", company360.LastContact{At: time.Date(2026, 11, 1, 22, 0, 0, 0, zone)}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := contactDays(&tc.contact, now, zone); got != tc.want {
				t.Fatalf("contactDays = %d, want %d", got, tc.want)
			}
		})
	}
}
