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

// The stored anchor reads back as its own date in every zone. That includes the
// zones a noon-UTC anchor puts on the wrong date.
func TestADateSurvivesItsAnchorInEveryZone(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Pacific/Auckland", "Pacific/Tongatapu", "Pacific/Kiritimati", "America/New_York", "Pacific/Pago_Pago", "UTC"} {
		loc := zone(t, name)
		for _, date := range []time.Time{
			time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC),   // a DST change in New York
			time.Date(2025, 4, 6, 0, 0, 0, 0, time.UTC),   // a DST change in Auckland
			time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), // a year end
		} {
			at := Anchor(date, loc)
			if got := Date(at, loc); !got.Equal(date) {
				t.Errorf("%s: %s anchored at %v reads back as %s", name, date.Format(time.DateOnly), at, got.Format(time.DateOnly))
			}
			if local := at.In(loc); local.Hour() != 12 {
				t.Errorf("%s: the anchor of %s is %v, want noon", name, date.Format(time.DateOnly), local)
			}
		}
	}
}

// The date is read off the value's own year, month and day, so a writer's
// offset names the date it meant.
func TestTheAnchorTakesTheDateInTheWritersOwnOffset(t *testing.T) {
	t.Parallel()
	honolulu := time.FixedZone("HST", -10*3600)
	lateEvening := time.Date(2025, 3, 9, 23, 0, 0, 0, honolulu)
	berlin := zone(t, "Europe/Berlin")
	want := time.Date(2025, 3, 9, 11, 0, 0, 0, time.UTC)
	if got := Anchor(lateEvening, berlin); !got.Equal(want) {
		t.Errorf("a Honolulu 9 March read for Berlin is anchored at %v, want %v", got, want)
	}
}

func TestWhenSaysADateOrAnInstant(t *testing.T) {
	t.Parallel()
	auckland := zone(t, "Pacific/Auckland")
	anchor := Anchor(time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC), auckland)
	if got := When(anchor, true, auckland); got != "2025-03-09" {
		t.Errorf("a day entry reads %q, want 2025-03-09", got)
	}
	if got := When(anchor, false, auckland); got != "2025-03-08T23:00:00Z" {
		t.Errorf("an instant reads %q, want its RFC 3339 form", got)
	}
}

// A day entry is not over until its date is.
func TestADayEntryEndsWithItsDate(t *testing.T) {
	t.Parallel()
	york := zone(t, "America/New_York")
	anchor := Anchor(time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC), york)
	for _, c := range []struct {
		asOf time.Time
		want bool
	}{
		{time.Date(2025, 3, 9, 8, 0, 0, 0, york), false},
		{time.Date(2025, 3, 9, 23, 59, 0, 0, york), false},
		{time.Date(2025, 3, 10, 0, 0, 0, 0, york), true},
	} {
		if got := Ended(anchor, c.asOf, york); got != c.want {
			t.Errorf("at %v, ended = %v, want %v", c.asOf, got, c.want)
		}
	}
}
