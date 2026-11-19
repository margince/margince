// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dueday

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
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

func TestOddNamesOnlyADayOutsideTheDueWindow(t *testing.T) {
	far := clocktest.Now(t).AddDate(15, 0, 0).Format(time.DateOnly)
	soon := clocktest.Now(t).AddDate(0, 0, 7).Format(time.DateOnly)
	for day, want := range map[string]string{far: far, soon: "", "": ""} {
		if got := Odd(day); got != want {
			t.Errorf("Odd(%q) = %q, want %q", day, got, want)
		}
	}
}
