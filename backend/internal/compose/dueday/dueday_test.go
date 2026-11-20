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

func TestEndOfComingRefusesADayThatIsOver(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for day, refused := range map[string]bool{"2026-11-19": true, "2026-11-20": false, "2026-11-21": false} {
		_, err := EndOfComing(context.Background(), nil, "review_on", day, "UTC", now)
		var detailed *httperr.DetailedError
		if got := errors.As(err, &detailed); got != refused {
			t.Errorf("EndOfComing(%q) refused = %v (%v); want %v", day, got, err, refused)
		}
	}
}
