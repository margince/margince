// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package closerisk

import (
	"testing"
	"time"
)

func TestACloseIsPassedBeforeTodayAndNearUpToTheHorizon(t *testing.T) {
	t.Parallel()
	for days, want := range map[int][2]bool{
		-1:                      {true, false},
		0:                       {false, true},
		RecoveryHorizonDays:     {false, true},
		RecoveryHorizonDays + 1: {false, false},
	} {
		if got := [2]bool{Passed(days), Near(days)}; got != want {
			t.Errorf("closes in %d days: passed, near = %v, want %v", days, got, want)
		}
	}
}

// 23:00 UTC on 14 May is already 15 May in Auckland. A deal due on the 14th is
// late there but not in UTC. A deal due on the 15th is late nowhere.
func TestACloseIsOverdueOnceItsDayIsOverInTheZone(t *testing.T) {
	t.Parallel()
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.May, 14, 23, 0, 0, 0, time.UTC)
	the14th, the15th := time.Date(2026, time.May, 14, 0, 0, 0, 0, time.UTC), time.Date(2026, time.May, 15, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		closes time.Time
		zone   *time.Location
		want   bool
	}{{the14th, auckland, true}, {the14th, time.UTC, false}, {the15th, auckland, false}} {
		if got := Overdue(c.closes, now, c.zone); got != c.want {
			t.Errorf("closing %s, in %s: overdue = %v, want %v", c.closes.Format(time.DateOnly), c.zone, got, c.want)
		}
	}
}
