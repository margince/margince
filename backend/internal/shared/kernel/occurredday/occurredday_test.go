// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package occurredday

import (
	"testing"
	"time"
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	return loc
}

// A day entry is stored at noon UTC of its date and reads back as that date.
func TestADayIsStoredAtNoonUTCAndReadsBackAsItsDate(t *testing.T) {
	t.Parallel()
	for _, date := range []time.Time{
		time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 4, 6, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	} {
		at := Anchor(date)
		if want := date.Add(12 * time.Hour); !at.Equal(want) || at.Location() != time.UTC {
			t.Errorf("%s is anchored at %v, want %v", date.Format(time.DateOnly), at, want)
		}
		if got := Date(at); !got.Equal(date) {
			t.Errorf("%s reads back as %s", date.Format(time.DateOnly), got.Format(time.DateOnly))
		}
		if got := When(at, true); got != date.Format(time.DateOnly) {
			t.Errorf("%s is told as %q", date.Format(time.DateOnly), got)
		}
	}
}

// The date is read off the value's own year, month and day, so a writer's
// offset names the date it meant.
func TestTheAnchorTakesTheDateInTheWritersOwnOffset(t *testing.T) {
	t.Parallel()
	honolulu := time.FixedZone("HST", -10*3600)
	lateEvening := time.Date(2025, 3, 9, 23, 0, 0, 0, honolulu)
	if got, want := Anchor(lateEvening), time.Date(2025, 3, 9, 12, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("a Honolulu 9 March is anchored at %v, want %v", got, want)
	}
}

// AnchorIn takes the date an instant falls on in a zone: the real local day.
func TestAnInstantIsDatedOnTheDayItFallsOnInTheZone(t *testing.T) {
	t.Parallel()
	instant := time.Date(2025, 3, 8, 23, 0, 0, 0, time.UTC)
	for name, want := range map[string]string{
		"Pacific/Auckland":    "2025-03-09",
		"UTC":                 "2025-03-08",
		"America/Los_Angeles": "2025-03-08",
	} {
		got := AnchorIn(instant, zone(t, name))
		if got.Format(time.DateOnly) != want || got.Hour() != 12 {
			t.Errorf("%s: %v is anchored at %v, want noon UTC of %s", name, instant, got, want)
		}
		if got := DateIn(instant, false, zone(t, name)).Format(time.DateOnly); got != want {
			t.Errorf("%s: an instant names %s, want %s", name, got, want)
		}
	}
}

func TestWhenSaysADateOrAnInstant(t *testing.T) {
	t.Parallel()
	anchor := Anchor(time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC))
	if got := When(anchor, true); got != "2025-03-09" {
		t.Errorf("a day entry reads %q, want 2025-03-09", got)
	}
	if got := When(anchor, false); got != "2025-03-09T12:00:00Z" {
		t.Errorf("an instant reads %q, want its RFC 3339 form", got)
	}
}

// A day entry is not over until its date is over in the zone that says which
// date is today. That holds east and west of UTC alike.
func TestADayEntryEndsWithItsDateInTheZone(t *testing.T) {
	t.Parallel()
	anchor := Anchor(time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC))
	for _, name := range []string{"America/New_York", "Pacific/Auckland", "Asia/Tokyo", "America/Los_Angeles", "Pacific/Kiritimati"} {
		loc := zone(t, name)
		for _, c := range []struct {
			asOf time.Time
			want bool
		}{
			{time.Date(2025, 3, 9, 0, 0, 0, 0, loc), false},
			{time.Date(2025, 3, 9, 23, 59, 0, 0, loc), false},
			{time.Date(2025, 3, 10, 0, 0, 0, 0, loc), true},
		} {
			if got := Ended(anchor, c.asOf, loc); got != c.want {
				t.Errorf("%s at %v: ended = %v, want %v", name, c.asOf, got, c.want)
			}
		}
	}
}

// A day entry's date begins at that date's midnight in the zone asked, even
// where its stored noon is already the next morning.
func TestADayBeginsAtItsOwnDatesMidnight(t *testing.T) {
	t.Parallel()
	anchor := Anchor(time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC))
	for _, name := range []string{"Pacific/Auckland", "America/Los_Angeles"} {
		loc := zone(t, name)
		if got, want := Begins(anchor, loc), time.Date(2026, 3, 10, 0, 0, 0, 0, loc); !got.Equal(want) {
			t.Errorf("%s: begins %v, want %v", name, got, want)
		}
	}
}

// A day entry has happened from the first minute of its date in the zone that
// says which date is today. That holds east and west of UTC, never a day early.
func TestADayEntryHasHappenedFromTheStartOfItsDate(t *testing.T) {
	t.Parallel()
	anchor := Anchor(time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC))
	for _, name := range []string{"Pacific/Auckland", "Asia/Tokyo", "UTC", "America/Los_Angeles", "Pacific/Kiritimati"} {
		loc := zone(t, name)
		for _, c := range []struct {
			asOf time.Time
			want bool
		}{
			{time.Date(2025, 3, 8, 23, 59, 0, 0, loc), false},
			{time.Date(2025, 3, 9, 0, 0, 0, 0, loc), true},
			{time.Date(2025, 3, 10, 9, 0, 0, 0, loc), true},
		} {
			if got := Begun(anchor, true, c.asOf, loc); got != c.want {
				t.Errorf("%s at %v: begun = %v, want %v", name, c.asOf, got, c.want)
			}
		}
	}
}

// An instant has happened once the clock reaches it, whatever the zone.
func TestAnInstantHasHappenedOnceTheClockReachesIt(t *testing.T) {
	t.Parallel()
	at := time.Date(2025, 3, 9, 12, 0, 0, 0, time.UTC)
	tokyo := zone(t, "Asia/Tokyo")
	if Begun(at, false, at.Add(-time.Minute), tokyo) {
		t.Error("an instant a minute ahead has happened")
	}
	if !Begun(at, false, at, tokyo) {
		t.Error("an instant at the clock has not happened")
	}
}

// A day entry dated today is zero days old, even where its stored noon is
// already tomorrow's local morning.
func TestADayEntryDatedTodayIsZeroDaysOld(t *testing.T) {
	t.Parallel()
	auckland := zone(t, "Pacific/Auckland")
	anchor := Anchor(time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC))
	morning := time.Date(2025, 3, 9, 9, 0, 0, 0, auckland)
	if got := DaysSince(anchor, true, morning, auckland); got != 0 {
		t.Errorf("a day entry dated today is %d days old, want 0", got)
	}
	if got := DaysSince(anchor, true, morning.AddDate(0, 0, 3), auckland); got != 3 {
		t.Errorf("three days on it is %d days old, want 3", got)
	}
	if got := DaysSince(morning.Add(-time.Hour), false, morning, auckland); got != 0 {
		t.Errorf("an instant an hour ago is %d days old, want 0", got)
	}
}
