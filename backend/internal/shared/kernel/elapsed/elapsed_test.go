// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package elapsed_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

func TestDaysCountsTheCalendarAndNotTheClock(t *testing.T) {
	// The case that made the two surfaces disagree: two hours apart, but on
	// either side of midnight. By the clock that is zero days; by the calendar,
	// which is how a reader looking at two dates counts, it is one.
	late := time.Date(2026, 5, 20, 23, 0, 0, 0, time.UTC)
	early := time.Date(2026, 5, 21, 1, 0, 0, 0, time.UTC)
	if got := elapsed.Days(late, early, time.UTC); got != 1 {
		t.Errorf("23:00 Monday to 01:00 Tuesday is 1 calendar day, got %d", got)
	}

	// And its mirror: nearly a full day apart, inside one calendar day.
	morning := time.Date(2026, 5, 20, 0, 30, 0, 0, time.UTC)
	night := time.Date(2026, 5, 20, 23, 30, 0, 0, time.UTC)
	if got := elapsed.Days(morning, night, time.UTC); got != 0 {
		t.Errorf("00:30 to 23:30 on one day is 0 calendar days, got %d", got)
	}
}

func TestDaysIsNegativeIntoTheFuture(t *testing.T) {
	// Callers tell a plan from an event by the sign. A count that clamped at
	// zero would read a booked meeting as having already happened.
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	later := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	if got := elapsed.Days(later, now, time.UTC); got != -3 {
		t.Errorf("counting back from a future moment is -3, got %d", got)
	}
}

// The same two instants are a different number of calendar days in different
// zones. Every pair is fixed, so no real clock is involved.
func TestDaysCountsInTheZoneTheSentenceIsDrawnIn(t *testing.T) {
	mustLoad := func(name string) *time.Location {
		t.Helper()
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		return loc
	}
	cases := []struct {
		name     string
		from, to time.Time
		loc      *time.Location
		want     int
	}{
		{"15:00 yesterday to 01:00 in UTC+7", time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC), time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC), mustLoad("Asia/Bangkok"), 1},
		{"the same pair in UTC", time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC), time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC), time.UTC, 0},
		{"15:00 yesterday to 01:00 in UTC-5", time.Date(2026, 12, 5, 20, 0, 0, 0, time.UTC), time.Date(2026, 12, 6, 6, 0, 0, 0, time.UTC), mustLoad("America/New_York"), 1},
		{"the evening before the UTC midnight in UTC-5", time.Date(2026, 12, 5, 23, 0, 0, 0, time.UTC), time.Date(2026, 12, 6, 1, 0, 0, 0, time.UTC), mustLoad("America/New_York"), 0},
		{"across the New York spring-forward", time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC), time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC), mustLoad("America/New_York"), 2},
		{"across the Berlin spring-forward", time.Date(2026, 3, 28, 23, 30, 0, 0, time.UTC), time.Date(2026, 3, 29, 22, 30, 0, 0, time.UTC), mustLoad("Europe/Berlin"), 1},
		{"a half-hour offset", time.Date(2026, 10, 6, 17, 30, 0, 0, time.UTC), time.Date(2026, 10, 6, 18, 45, 0, 0, time.UTC), mustLoad("Asia/Kolkata"), 1},
		{"a thirty-minute DST zone", time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC), time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC), mustLoad("Australia/Lord_Howe"), 1},
		{"a future instant is negative", time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), time.UTC, -3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := elapsed.Days(tc.from, tc.to, tc.loc); got != tc.want {
				t.Errorf("Days = %d, want %d", got, tc.want)
			}
		})
	}
}

// A zone-carrying timestamp counts the same as the instant it names. Reading
// the server's local wall-clock fields would make the answer depend on where
// the process runs.
func TestDaysIgnoresTheZoneTheMomentsAreCarriedIn(t *testing.T) {
	carried := time.FixedZone("UTC+7", 7*60*60)
	from := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	to := from.Add(48 * time.Hour).In(carried)
	if got := elapsed.Days(from, to, time.UTC); got != 2 {
		t.Errorf("48 hours is 2 days however the second moment is zoned, got %d", got)
	}
}

func TestCalendarDayIsTheLocalDateAtUTCMidnight(t *testing.T) {
	bangkok := time.FixedZone("UTC+7", 7*60*60)
	got := elapsed.CalendarDay(time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC), bangkok)
	want := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("CalendarDay = %v, want %v", got, want)
	}
}

// A deadline counts differently from a pair of dates. These pin the boundary
// where the two rules disagree, which is where the bug was: a promise due in
// two hours, read across UTC midnight.
func TestFullDaysUntilCountsRemainingTimeNotCalendarBoundaries(t *testing.T) {
	// 23:00 UTC, due 01:00 the next day: one calendar boundary, no whole day.
	now := time.Date(2026, 9, 1, 23, 0, 0, 0, time.UTC)
	soon := now.Add(2 * time.Hour)
	if got := elapsed.FullDaysUntil(now, soon); got != 0 {
		t.Errorf("FullDaysUntil = %d for a deadline two hours away, want 0 — it reads as \"due today\"", got)
	}
	if got := elapsed.Days(now, soon, time.UTC); got != 1 {
		t.Fatalf("Days = %d, want 1; without the calendar rule differing here these two would be one function", got)
	}
	if got := elapsed.FullDaysUntil(now, now.Add(50*time.Hour)); got != 2 {
		t.Errorf("FullDaysUntil = %d for 50 hours out, want 2", got)
	}
	// A deadline already past is not "in -1 days"; DaysPast answers that.
	if got := elapsed.FullDaysUntil(now, now.Add(-time.Hour)); got != 0 {
		t.Errorf("FullDaysUntil = %d for a past deadline, want 0", got)
	}
}
