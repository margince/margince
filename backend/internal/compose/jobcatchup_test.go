// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/riverqueue/river"
)

var catchUpNow = time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)

// A pass that ran within its interval waits until one interval after that
// run, plus its fixed spread (#11026).
func TestAPassThatRanRecentlyWaitsOutItsInterval(t *testing.T) {
	lastRun := catchUpNow.Add(-time.Hour)
	s := newCatchUpSchedule("daily_pass", river.PeriodicInterval(24*time.Hour), lastRun, startupSpread)
	want := lastRun.Add(24 * time.Hour).Add(startupDelay("daily_pass", 24*time.Hour, startupSpread))
	if got := s.Next(catchUpNow); !got.Equal(want) {
		t.Fatalf("first run at %s, want %s (one interval after the last run, plus the spread)", got, want)
	}
}

// An overdue pass, or one with no recorded run, starts within its spread.
func TestAnOverdueOrUnrecordedPassRunsWithinItsSpread(t *testing.T) {
	for name, lastRun := range map[string]time.Time{
		"overdue":      catchUpNow.Add(-25 * time.Hour),
		"no record":    {},
		"just now due": catchUpNow.Add(-24 * time.Hour),
	} {
		s := newCatchUpSchedule("daily_pass", river.PeriodicInterval(24*time.Hour), lastRun, startupSpread)
		got := s.Next(catchUpNow)
		if got.Before(catchUpNow) || !got.Before(catchUpNow.Add(startupSpread)) {
			t.Errorf("%s: first run at %s, want within [%s, %s)", name, got, catchUpNow, catchUpNow.Add(startupSpread))
		}
	}
}

// After the first run the schedule is the plain interval, and rearm brings the
// catch-up back for a schedule River places again.
func TestOnlyTheFirstNextCatchesUpUntilRearmed(t *testing.T) {
	s := newCatchUpSchedule("hourly_pass", river.PeriodicInterval(time.Hour), catchUpNow.Add(-10*time.Minute), startupSpread)
	first := s.Next(catchUpNow)
	if got, want := s.Next(first), first.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("second run at %s, want %s (plain interval)", got, want)
	}
	s.rearm()
	if got := s.Next(catchUpNow); !got.Equal(first) {
		t.Fatalf("after rearm the first run is %s, want the catch-up %s again", got, first)
	}
}

// The spread stays inside a short pass's interval and is fixed per kind. It
// differs between kinds, so a start does not put every pass in one minute.
func TestTheStartupDelayIsBoundedAndSpreadsKinds(t *testing.T) {
	if d := startupDelay("fast_pass", 30*time.Second, startupSpread); d < 0 || d >= 30*time.Second {
		t.Fatalf("delay %s for a 30s pass, want within its interval", d)
	}
	first := startupDelay("a_pass", time.Hour, startupSpread)
	if again := startupDelay("a_pass", time.Hour, startupSpread); again != first {
		t.Fatalf("the delay for one kind changed between calls: %s then %s", first, again)
	}
	if startupDelay("any_pass", time.Hour, 0) != 0 {
		t.Fatal("a zero spread must mean no delay")
	}
	seen := map[time.Duration]bool{}
	for _, kind := range []string{"privacy_retention", "participant_backfill", "dedupe_recheck", "close_date_sweep", "weekly_review_generate"} {
		seen[startupDelay(kind, 24*time.Hour, startupSpread)/time.Minute] = true
	}
	if len(seen) < 2 {
		t.Fatalf("five kinds all start in the same minute: %v", seen)
	}
}

// A book without run history keeps RunOnStart; one given history does not.
func TestABookCatchesUpOnlyOnceGivenHistory(t *testing.T) {
	book := newScheduleBook(nil)
	if _, s := book.periodicJob("a_pass", "", river.PeriodicInterval(time.Hour), nil); s != nil {
		t.Fatal("a book with no history built a catch-up schedule")
	}
	book.CatchUpFrom(map[string]time.Time{"a_pass": catchUpNow})
	_, s := book.periodicJob("a_pass", "", river.PeriodicInterval(time.Hour), nil)
	if s == nil || !s.lastRun.Equal(catchUpNow) || s.spread != startupSpread {
		t.Fatalf("a book with history built %+v, want a catch-up from %s", s, catchUpNow)
	}
}
