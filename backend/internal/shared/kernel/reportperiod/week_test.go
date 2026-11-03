// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportperiod

import (
	"testing"
	"time"
)

func TestWeekStartIsLocalMondayMidnightAcrossDSTAndMonthEnds(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		at   time.Time
		zone *time.Location
		want string
	}{
		{"monday midnight is its own week", time.Date(2026, 10, 26, 0, 0, 0, 0, berlin), berlin, "2026-10-26T00:00:00+01:00"},
		{"sunday night is still the week before", time.Date(2026, 10, 25, 23, 59, 59, 0, berlin), berlin, "2026-10-19T00:00:00+02:00"},
		{"the week the clocks go back", time.Date(2026, 10, 29, 9, 0, 0, 0, berlin), berlin, "2026-10-26T00:00:00+01:00"},
		{"the week the clocks go forward", time.Date(2026, 3, 29, 12, 0, 0, 0, berlin), berlin, "2026-03-23T00:00:00+01:00"},
		{"a week that began last month", time.Date(2026, 11, 1, 10, 0, 0, 0, berlin), berlin, "2026-10-26T00:00:00+01:00"},
		{"a UTC instant already on Monday east of it", time.Date(2026, 10, 25, 12, 30, 0, 0, time.UTC), auckland, "2026-10-26T00:00:00+13:00"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := WeekStart(test.at, test.zone).Format(time.RFC3339); got != test.want {
				t.Fatalf("WeekStart = %s, want %s", got, test.want)
			}
		})
	}
}
