// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dueday

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestTheDayAfterAPromisedDayIsTheNextCalendarDay(t *testing.T) {
	for day, want := range map[string]string{
		"2026-03-11": "2026-03-12",
		"2026-02-28": "2026-03-01",
		"2026-12-31": "2027-01-01",
		"":           "",
	} {
		got, err := DayAfter(day)
		if err != nil || got != want {
			t.Errorf("DayAfter(%q) = %q, %v; want %q", day, got, err, want)
		}
	}
	if _, err := DayAfter("Friday"); err == nil {
		t.Error("a day that is not YYYY-MM-DD must be refused, not followed up on a guessed day")
	}
}

func TestOddClassifiesTheExactEndOfTheDayAtBothEdgesOfTheWindow(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for day, want := range map[string]string{
		"2036-11-19": "", "2036-11-20": "2036-11-20",
		"2016-11-20": "", "2016-11-19": "2016-11-19",
		"": "",
	} {
		got, err := Odd(context.Background(), nil, day, "UTC", now)
		if err != nil || got != want {
			t.Errorf("Odd(%q) = %q, %v; want %q", day, got, err, want)
		}
	}
}

func TestOddReadsTheDayInTheWritersZone(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	got, err := Odd(context.Background(), nil, "2036-11-20", "Pacific/Auckland", now)
	if err != nil || got != "" {
		t.Errorf("the day ends before the edge in Auckland: Odd = %q, %v; want none", got, err)
	}
}

func TestComingDayRefusesADayOnlyOnceItIsOverInTheZone(t *testing.T) {
	cases := []struct {
		name, day, zone string
		now             time.Time
		refused         bool
	}{
		{"yesterday", "2026-11-19", "UTC", time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC), true},
		{"today", "2026-11-20", "UTC", time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC), false},
		{"tomorrow", "2026-11-21", "UTC", time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC), false},
		{"last fraction of today", "2026-11-20", "UTC", time.Date(2026, 11, 20, 23, 59, 59, 500_000_000, time.UTC), false},
		{"already tomorrow in Auckland", "2026-11-20", "Pacific/Auckland", time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC), true},
		{"still the day before in Los Angeles", "2026-11-19", "America/Los_Angeles", time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC), false},
	}
	for _, c := range cases {
		intent, err := ComingDay(context.Background(), nil, "review_on", c.day, c.zone, c.now)
		var detailed *httperr.DetailedError
		if got := errors.As(err, &detailed); got != c.refused {
			t.Errorf("%s: refused = %v (%v); want %v", c.name, got, err, c.refused)
		}
		if !c.refused && (intent.WallClock != c.day+"T23:59:59" || intent.Timezone != c.zone) {
			t.Errorf("%s: intent = %+v; want the day's last second in %s", c.name, intent, c.zone)
		}
	}
}
