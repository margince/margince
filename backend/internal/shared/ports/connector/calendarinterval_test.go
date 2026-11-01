// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"testing"
	"time"
)

func TestCalendarOccupancyUsesInstantsAndRejectsReversedBounds(t *testing.T) {
	start := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name              string
		end               time.Time
		occupied, invalid bool
	}{
		{"normal event", start.Add(time.Hour), true, false},
		{"placeholder", start, false, false},
		{"same instant in another offset", start.In(time.FixedZone("offset", 7*3600)), false, false},
		{"reversed event", start.Add(-time.Minute), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			occupied, err := CalendarOccupancy(start, tc.end)
			if occupied != tc.occupied || (err != nil) != tc.invalid {
				t.Fatalf("occupied=%v err=%v", occupied, err)
			}
		})
	}
}
