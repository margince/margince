// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dormancy

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

var berlin = mustZone("Europe/Berlin")

func mustZone(name string) *time.Location {
	zone, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return zone
}

// now is a fixed afternoon in Berlin. The last activity in each case is the
// morning of a day that many calendar days earlier.
var now = time.Date(2026, 11, 20, 15, 0, 0, 0, berlin)

func daysAgo(days int) *occurredday.Moment {
	return &occurredday.Moment{At: time.Date(2026, 11, 20, 9, 0, 0, 0, berlin).AddDate(0, 0, -days)}
}

func TestEachRungBeginsOnItsDay(t *testing.T) {
	t.Parallel()
	cases := []struct {
		days int
		want Rung
	}{
		{0, Active},
		{29, Active},
		{30, Cooling},
		{59, Cooling},
		{60, Dormant},
		{89, Dormant},
		{90, Stale},
		{400, Stale},
	}
	for _, tc := range cases {
		got := Default.Read(daysAgo(tc.days), now, berlin)
		if got.Rung != tc.want || got.SilentDays != tc.days || got.NoActivity {
			t.Errorf("%d silent days read as %+v, want %s", tc.days, got, tc.want)
		}
	}
}

// A day boundary is the installation zone's midnight, not UTC's. 23:30 in
// Berlin on the 30th day back is still that day, though UTC calls it the next.
func TestTheDayCountFollowsTheInstallationZone(t *testing.T) {
	t.Parallel()
	late := occurredday.Moment{At: time.Date(2026, 10, 21, 23, 30, 0, 0, berlin)}
	if got := Default.Read(&late, now, berlin); got.Rung != Cooling || got.SilentDays != 30 {
		t.Errorf("a late evening 30 days back read as %+v, want cooling on day 30", got)
	}
}

func TestAnAccountWithNoActivityReadsAsStaleAndSaysWhy(t *testing.T) {
	t.Parallel()
	got := Default.Read(nil, now, berlin)
	if got.Rung != Stale || !got.NoActivity || got.SilentDays != 0 {
		t.Errorf("no activity read as %+v, want stale with the no-activity marker", got)
	}
}

// Activity dated after now, such as a meeting booked for later today, is not
// silence.
func TestALaterActivityReadsAsActive(t *testing.T) {
	t.Parallel()
	later := occurredday.Moment{At: now.Add(48 * time.Hour)}
	if got := Default.Read(&later, now, berlin); got.Rung != Active || got.SilentDays != 0 {
		t.Errorf("a later activity read as %+v, want active with no silent days", got)
	}
}

func TestTheLadderTakesOnlyThreeAscendingDays(t *testing.T) {
	t.Parallel()
	if got, err := FromDays([]int{14, 45, 3650}); err != nil || got != (Ladder{14, 45, 3650}) {
		t.Errorf("FromDays(14, 45, 3650) = %+v, %v", got, err)
	}
	for _, days := range [][]int{nil, {30, 60}, {30, 60, 90, 120}, {0, 60, 90}, {30, 30, 90}, {60, 30, 90}, {30, 90, 60}, {30, 60, 3651}} {
		if _, err := FromDays(days); err == nil {
			t.Errorf("FromDays(%v) was accepted, want refused", days)
		}
	}
	if got, err := FromDays(DefaultDays()); err != nil || got != Default {
		t.Errorf("the default does not read back as itself: %+v, %v", got, err)
	}
}

// The list filter and the page must put an account on the same rung. The
// window's days back are read here as the SQL reads them. A last activity d
// days back is inside when d is at most FromDaysBack and more than
// BeforeDaysBack. The SQL itself is checked against Read on the boundary days
// by the company list's integration test.
func TestTheFilterWindowAgreesWithTheReading(t *testing.T) {
	t.Parallel()
	ladders := []Ladder{Default, {Cooling: 1, Dormant: 2, Stale: 3}, {Cooling: 7, Dormant: 8, Stale: 200}}
	for _, ladder := range ladders {
		for days := 0; days <= 220; days++ {
			want := ladder.Read(daysAgo(days), now, berlin).Rung
			for _, rung := range Rungs() {
				w := ladder.WindowOf(rung)
				got := (w.FromDaysBack == nil || days <= *w.FromDaysBack) && (w.BeforeDaysBack == nil || days > *w.BeforeDaysBack)
				if got != (rung == want) {
					t.Fatalf("ladder %v, %d days back: the %s window admits=%v, but the reading is %s", ladder, days, rung, got, want)
				}
			}
		}
		for _, rung := range Rungs() {
			if got := ladder.WindowOf(rung).IncludesNone; got != (rung == Stale) {
				t.Errorf("ladder %v: the %s window admits no activity = %v", ladder, rung, got)
			}
		}
	}
}

func TestOnlyDormantAndStaleAreCold(t *testing.T) {
	t.Parallel()
	for _, rung := range Rungs() {
		if got, want := rung.Cold(), rung == Dormant || rung == Stale; got != want {
			t.Errorf("%s.Cold() = %v, want %v", rung, got, want)
		}
	}
}

// A date-only last activity counts its silence from its own date. Its noon-UTC
// anchor is already the next day at UTC+14, which read the account a day warmer.
func TestADateOnlyLastActivityCountsFromItsOwnDate(t *testing.T) {
	t.Parallel()
	kiritimati, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	stored := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 11, 25, 10, 30, 0, 0, time.UTC)
	day := Default.Read(&occurredday.Moment{At: stored, DateOnly: true}, asOf, kiritimati)
	instant := Default.Read(&occurredday.Moment{At: stored}, asOf, kiritimati)
	if day.SilentDays != 6 || instant.SilentDays != 5 {
		t.Errorf("silent days = %d for the date-only activity and %d for the instant, want 6 and 5",
			day.SilentDays, instant.SilentDays)
	}
}

// The loosened window is the indexed pre-filter on the stored clock. A date-only
// clock is stored up to a day and a half after its date's start. So the later
// bound moves by DateStartLead days, and the earlier one stays.
func TestTheLoosenedWindowAdmitsEveryClockTheWindowDoes(t *testing.T) {
	t.Parallel()
	for _, rung := range Rungs() {
		w := Default.WindowOf(rung)
		got := w.Loosened()
		if (w.FromDaysBack == nil) != (got.FromDaysBack == nil) || (w.FromDaysBack != nil && *w.FromDaysBack != *got.FromDaysBack) {
			t.Errorf("%s: the loosened window moved its earlier bound: %v -> %v", rung, w.FromDaysBack, got.FromDaysBack)
		}
		if w.BeforeDaysBack != nil && *got.BeforeDaysBack != *w.BeforeDaysBack-DateStartLead {
			t.Errorf("%s: the later bound is %d days back, want %d", rung, *got.BeforeDaysBack, *w.BeforeDaysBack-DateStartLead)
		}
	}
}
