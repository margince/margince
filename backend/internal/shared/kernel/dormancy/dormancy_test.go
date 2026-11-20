// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dormancy

import (
	"testing"
	"time"
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

func daysAgo(days int) *time.Time {
	at := time.Date(2026, 11, 20, 9, 0, 0, 0, berlin).AddDate(0, 0, -days)
	return &at
}

func TestEachRungBeginsOnItsDay(t *testing.T) {
	t.Parallel()
	cases := []struct {
		days int
		want Rung
	}{
		{0, Active}, {29, Active},
		{30, Cooling}, {59, Cooling},
		{60, Dormant}, {89, Dormant},
		{90, Stale}, {400, Stale},
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
	late := time.Date(2026, 10, 21, 23, 30, 0, 0, berlin)
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
	later := now.Add(48 * time.Hour)
	if got := Default.Read(&later, now, berlin); got.Rung != Active || got.SilentDays != 0 {
		t.Errorf("a later activity read as %+v, want active with no silent days", got)
	}
}

func TestTheLadderTakesOnlyThreeAscendingDays(t *testing.T) {
	t.Parallel()
	if got, err := FromDays([]int{14, 45, 120}); err != nil || got != (Ladder{14, 45, 120}) {
		t.Errorf("FromDays(14, 45, 120) = %+v, %v", got, err)
	}
	for _, days := range [][]int{nil, {30, 60}, {30, 60, 90, 120}, {0, 60, 90}, {30, 30, 90}, {60, 30, 90}, {30, 90, 60}} {
		if _, err := FromDays(days); err == nil {
			t.Errorf("FromDays(%v) was accepted, want refused", days)
		}
	}
	if got, err := FromDays(DefaultDays()); err != nil || got != Default {
		t.Errorf("the default does not read back as itself: %+v, %v", got, err)
	}
}

// The list filter and the page must put an account on the same rung. Each
// day back is checked at both ends of the day, across a DST change.
func TestTheFilterWindowAgreesWithTheReading(t *testing.T) {
	t.Parallel()
	ladders := []Ladder{Default, {Cooling: 1, Dormant: 2, Stale: 3}, {Cooling: 7, Dormant: 8, Stale: 200}}
	for _, ladder := range ladders {
		for days := -2; days <= 220; days++ {
			day := now.AddDate(0, 0, -days)
			for _, at := range []time.Time{
				time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, berlin),
				time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, berlin),
			} {
				want := ladder.Read(&at, now, berlin).Rung
				for _, rung := range Rungs() {
					if got := ladder.WindowOf(rung).admits(&at); got != (rung == want) {
						t.Fatalf("ladder %v, %s: the %s window admits=%v, but the reading is %s", ladder, at, rung, got, want)
					}
				}
			}
		}
		for _, rung := range Rungs() {
			if got := ladder.WindowOf(rung).admits(nil); got != (rung == Stale) {
				t.Errorf("ladder %v: the %s window admits no activity = %v", ladder, rung, got)
			}
		}
	}
}

func (w Window) admits(last *time.Time) bool {
	if last == nil {
		return w.IncludesNone
	}
	from, before := w.Bounds(now, berlin)
	return (from == nil || !last.Before(*from)) && (before == nil || last.Before(*before))
}

func TestOnlyDormantAndStaleAreCold(t *testing.T) {
	t.Parallel()
	for _, rung := range Rungs() {
		if got, want := rung.Cold(), rung == Dormant || rung == Stale; got != want {
			t.Errorf("%s.Cold() = %v, want %v", rung, got, want)
		}
	}
}
