// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"testing"
	"time"
)

func TestDueInWindowRefusesAbsurdYearsAndKeepsRealOnes(t *testing.T) {
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		due  time.Time
		want bool
	}{
		"1900":              {time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC), false},
		"9999":              {time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), false},
		"just past 10 back": {now.AddDate(-10, 0, -1), false},
		"just past 10 on":   {now.AddDate(10, 0, 1), false},
		"two years back":    {now.AddDate(-2, 0, 0), true},
		"next month":        {now.AddDate(0, 1, 0), true},
	}
	for name, c := range cases {
		if got := DueInWindow(c.due, now); got != c.want {
			t.Errorf("%s: DueInWindow = %v, want %v", name, got, c.want)
		}
	}
}

func TestDueDayInWindowJudgesAStatedDay(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for day, want := range map[string]bool{
		"2026-12-01": true, "2046-12-01": false, "1999-01-01": false, "not a day": true,
	} {
		if got := DueDayInWindow(day, now); got != want {
			t.Errorf("DueDayInWindow(%q) = %v, want %v", day, got, want)
		}
	}
}
