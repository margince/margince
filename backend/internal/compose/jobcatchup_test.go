// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/clocktime"
	"github.com/margince/margince/backend/pkg/extension"
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

func berlin(t *testing.T) *time.Location {
	t.Helper()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return zone
}

var weekdaysAt0730 = extension.TimeOfDay{
	Minute: 7*60 + 30,
	Days:   []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
}

func clockAt(at time.Time) func() time.Time { return func() time.Time { return at } }

// riverRuns replays River's loop for a stored slot restored after downtime.
// River enqueues the slot and asks Next with it. It enqueues again while the
// answer is already due.
func riverRuns(s river.PeriodicSchedule, restored, now time.Time) (runs int, next time.Time) {
	for next = restored; !next.After(now); next = s.Next(next) {
		runs++
	}
	return runs, next
}

// Three days down: River restores Monday's 07:30 and must not then replay
// Tuesday and Wednesday as well.
func TestARestoredPastSlotRunsOnceAndThenWaitsForTheNextSlot(t *testing.T) {
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC) // Thursday
	restored := time.Date(2026, 11, 23, 7, 30, 0, 0, time.UTC)
	tomorrow := time.Date(2026, 11, 27, 7, 30, 0, 0, time.UTC)
	every := clocktime.Schedule{At: extension.TimeOfDay{Minute: 7*60 + 30}, Zone: func() *time.Location { return time.UTC }, Now: clockAt(now)}
	runs, next := riverRuns(newCatchUpSchedule("ext_demo_report", every, restored.Add(-24*time.Hour), startupSpread), restored, now)
	if runs != 1 || !next.Equal(tomorrow) {
		t.Errorf("%d runs, then next at %s; want 1 run, then %s", runs, next, tomorrow)
	}
}

func TestAClockTimePassCatchesUpOnlyAMissedSlot(t *testing.T) {
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	every := clocktime.Schedule{At: extension.TimeOfDay{Minute: 7*60 + 30}, Zone: func() *time.Location { return time.UTC }, Now: clockAt(now)}
	todaySlot, tomorrowSlot := time.Date(2026, 11, 25, 7, 30, 0, 0, time.UTC), time.Date(2026, 11, 26, 7, 30, 0, 0, time.UTC)

	missed := newCatchUpSchedule("ext_demo_report", every, todaySlot.Add(-24*time.Hour), startupSpread)
	if got := missed.Next(now); got.Before(now) || !got.Before(now.Add(startupSpread)) {
		t.Errorf("after a missed slot the first run is %s, want within the spread from %s", got, now)
	}
	ran := newCatchUpSchedule("ext_demo_report", every, todaySlot, startupSpread)
	if got := ran.Next(now); !got.Equal(tomorrowSlot) {
		t.Errorf("after today's run the first run is %s, want tomorrow's slot %s", got, tomorrowSlot)
	}
	never := newCatchUpSchedule("ext_demo_report", every, time.Time{}, startupSpread)
	if got := never.Next(now); !got.Equal(tomorrowSlot) {
		t.Errorf("a pass that never ran first runs %s, want its next slot %s", got, tomorrowSlot)
	}
}

func TestAClockTimePassNeverRunsAtStartEvenWithoutHistory(t *testing.T) {
	book := newScheduleBook(nil)
	book.clock = clockAt(time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC))
	periodicForComposed(book, extension.JobDeclaration{Unit: "demo", Job: "report", At: &weekdaysAt0730})
	catchUp := book.kinds["ext_demo_report"].catchUp
	if catchUp == nil {
		t.Fatal("a clock-time pass in a book with no history got RunOnStart instead of waiting for its slot")
	}
	// 2026-11-25 is a Wednesday.
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	if got, want := catchUp.Next(now), time.Date(2026, 11, 26, 7, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("first run %s, want the next slot %s", got, want)
	}
}

func TestAChangedZoneMovesTheClockTimePassesWithoutARun(t *testing.T) {
	book := newScheduleBook(nil)
	periodicForComposed(book, extension.JobDeclaration{Unit: "demo", Job: "report", At: &weekdaysAt0730})
	periodicForComposed(book, extension.JobDeclaration{Unit: "demo", Job: "sync", Cadence: time.Hour})
	runner := &periodicLog{}
	if err := book.applyZone(time.UTC, runner); err != nil || len(runner.calls) != 0 {
		t.Fatalf("an unchanged zone asked the runner %v (err %v), want nothing", runner.calls, err)
	}
	if err := book.applyZone(berlin(t), runner); err != nil {
		t.Fatal(err)
	}
	if want := []string{"remove ext_demo_report", "add"}; !slices.Equal(runner.calls, want) {
		t.Errorf("runner was asked %v, want %v", runner.calls, want)
	}
	if book.location().String() != "Europe/Berlin" {
		t.Errorf("the book counts in %s, want Europe/Berlin", book.location())
	}
}

func TestAZoneMoveTheRunnerRefusedIsRetried(t *testing.T) {
	book := newScheduleBook(nil)
	periodicForComposed(book, extension.JobDeclaration{Unit: "demo", Job: "report", At: &weekdaysAt0730})
	if err := book.applyZone(berlin(t), &periodicLog{failAdds: 1}); err == nil {
		t.Fatal("a refused add reported success")
	}
	runner := &periodicLog{}
	if err := book.applyZone(berlin(t), runner); err != nil || len(runner.calls) != 2 {
		t.Errorf("the retry asked the runner %v (err %v), want the pass moved again", runner.calls, err)
	}
}

func TestOnlyAClockTimeScheduleRunsAtClockTimes(t *testing.T) {
	if runsAtClockTimes(river.PeriodicInterval(time.Hour)) || !runsAtClockTimes(clocktime.Schedule{}) {
		t.Error("runsAtClockTimes must tell an interval from a clock time")
	}
}
