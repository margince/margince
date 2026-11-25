// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package clocktime

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return zone
}

func clockAt(at time.Time) func() time.Time { return func() time.Time { return at } }

var weekdaysAt0730 = extension.TimeOfDay{
	Minute: 7*60 + 30,
	Days:   []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
}

// Three days down: River restores Monday's 07:30, then asks Next with it. The
// answer is tomorrow's slot, not Tuesday's or Wednesday's.
func TestARestoredPastSlotIsFollowedByTheNextSlotAfterNow(t *testing.T) {
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC) // Thursday
	s := Schedule{At: extension.TimeOfDay{Minute: 7*60 + 30}, Zone: func() *time.Location { return time.UTC }, Now: clockAt(now)}
	if got, want := s.Next(time.Date(2026, 11, 23, 7, 30, 0, 0, time.UTC)), time.Date(2026, 11, 27, 7, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Next of the restored slot = %s, want %s", got, want)
	}
}

func TestAClockTimeScheduleRunsAtTheNextLocalSlot(t *testing.T) {
	zone := berlin(t)
	at := func(y int, m time.Month, d, h, minute int) time.Time {
		return time.Date(y, m, d, h, minute, 0, 0, zone)
	}
	daily := extension.TimeOfDay{Minute: 7*60 + 30}
	for name, tc := range map[string]struct {
		at      extension.TimeOfDay
		current time.Time
		want    time.Time
	}{
		// 2026-11-25 is a Wednesday.
		"daily, before today's slot":  {daily, at(2026, 11, 25, 6, 0), at(2026, 11, 25, 7, 30)},
		"daily, at today's slot":      {daily, at(2026, 11, 25, 7, 30), at(2026, 11, 26, 7, 30)},
		"weekdays, on a friday after": {weekdaysAt0730, at(2026, 11, 27, 8, 0), at(2026, 11, 30, 7, 30)},
		"weekdays, on a saturday":     {weekdaysAt0730, at(2026, 11, 28, 6, 0), at(2026, 11, 30, 7, 30)},
		// Clocks spring from 02:00 to 03:00 on 2026-03-29.
		"a time the clock skips": {extension.TimeOfDay{Minute: 2*60 + 30}, at(2026, 3, 29, 1, 0), at(2026, 3, 29, 3, 0)},
	} {
		s := Schedule{At: tc.at, Zone: func() *time.Location { return zone }, Now: clockAt(time.Time{})}
		if got := s.Next(tc.current); !got.Equal(tc.want) {
			t.Errorf("%s: Next(%s) = %s, want %s", name, tc.current, got, tc.want)
		}
	}
}

// Clocks fall back from 03:00 to 02:00 on 2026-10-25, so 02:30 happens twice.
func TestATimeTheClockShowsTwiceRunsOnce(t *testing.T) {
	zone := berlin(t)
	s := Schedule{At: extension.TimeOfDay{Minute: 2*60 + 30}, Zone: func() *time.Location { return zone }, Now: clockAt(time.Time{})}
	first := s.Next(time.Date(2026, 10, 25, 0, 0, 0, 0, zone))
	if h, m, _ := first.In(zone).Clock(); h != 2 || m != 30 || first.In(zone).Day() != 25 {
		t.Fatalf("first run %s, want 02:30 on the 25th", first.In(zone))
	}
	if second := s.Next(first); second.In(zone).Day() != 26 {
		t.Errorf("after the 02:30 run the next is %s, want the 26th", second.In(zone))
	}
}

func TestAClockTimeScheduleCountsInTheZoneItIsGivenEachTime(t *testing.T) {
	zone := time.UTC
	s := Schedule{At: extension.TimeOfDay{Minute: 7*60 + 30}, Zone: func() *time.Location { return zone }, Now: clockAt(time.Time{})}
	current := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	if got := s.Next(current); !got.Equal(time.Date(2026, 11, 25, 7, 30, 0, 0, time.UTC)) {
		t.Fatalf("in UTC: %s", got)
	}
	zone = berlin(t)
	if got := s.Next(current); !got.Equal(time.Date(2026, 11, 25, 6, 30, 0, 0, time.UTC)) {
		t.Errorf("after the zone moved to Berlin: %s, want 06:30 UTC", got)
	}
}
