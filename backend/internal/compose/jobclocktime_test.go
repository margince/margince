// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/pkg/extension"
)

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
	every := clockTimeSchedule{at: extension.TimeOfDay{Minute: 7*60 + 30}, zone: func() *time.Location { return time.UTC }, now: clockAt(now)}
	for name, schedule := range map[string]river.PeriodicSchedule{
		"the bare schedule": every,
		"behind a catch-up": newCatchUpSchedule("ext_demo_report", every, restored.Add(-24*time.Hour), startupSpread),
	} {
		runs, next := riverRuns(schedule, restored, now)
		if runs != 1 || !next.Equal(tomorrow) {
			t.Errorf("%s: %d runs, then next at %s; want 1 run, then %s", name, runs, next, tomorrow)
		}
	}
}

func TestAClockTimeScheduleRunsAtTheNextLocalSlot(t *testing.T) {
	zone := berlin(t)
	at := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, zone) }
	daily := extension.TimeOfDay{Minute: 7*60 + 30}
	for name, tc := range map[string]struct {
		at      extension.TimeOfDay
		current time.Time
		want    time.Time
	}{
		// 2026-11-25 is a Wednesday.
		"daily, before today's slot":  {daily, at(2026, 11, 25, 6, 0), at(2026, 11, 25, 7, 30)},
		"daily, at today's slot":      {daily, at(2026, 11, 25, 7, 30), at(2026, 11, 26, 7, 30)},
		"weekdays, on a friday after": {weekdaysAt0730, at(2026, 11, 27, 8, 0), at(2026, 11, 30, 7, 30)},
		"weekdays, on a saturday":     {weekdaysAt0730, at(2026, 11, 28, 6, 0), at(2026, 11, 30, 7, 30)},
		// Clocks spring from 02:00 to 03:00 on 2026-03-29.
		"a time the clock skips": {extension.TimeOfDay{Minute: 2*60 + 30}, at(2026, 3, 29, 1, 0), at(2026, 3, 29, 3, 0)},
	} {
		s := clockTimeSchedule{at: tc.at, zone: func() *time.Location { return zone }, now: clockAt(time.Time{})}
		if got := s.Next(tc.current); !got.Equal(tc.want) {
			t.Errorf("%s: Next(%s) = %s, want %s", name, tc.current, got, tc.want)
		}
	}
}

// Clocks fall back from 03:00 to 02:00 on 2026-10-25, so 02:30 happens twice.
func TestATimeTheClockShowsTwiceRunsOnce(t *testing.T) {
	zone := berlin(t)
	s := clockTimeSchedule{at: extension.TimeOfDay{Minute: 2*60 + 30}, zone: func() *time.Location { return zone }, now: clockAt(time.Time{})}
	first := s.Next(time.Date(2026, 10, 25, 0, 0, 0, 0, zone))
	if h, m, _ := first.In(zone).Clock(); h != 2 || m != 30 || first.In(zone).Day() != 25 {
		t.Fatalf("first run %s, want 02:30 on the 25th", first.In(zone))
	}
	if second := s.Next(first); second.In(zone).Day() != 26 {
		t.Errorf("after the 02:30 run the next is %s, want the 26th", second.In(zone))
	}
}

func TestAClockTimeScheduleCountsInTheZoneItIsGivenEachTime(t *testing.T) {
	zone := time.UTC
	s := clockTimeSchedule{at: extension.TimeOfDay{Minute: 7*60 + 30}, zone: func() *time.Location { return zone }, now: clockAt(time.Time{})}
	current := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	if got := s.Next(current); !got.Equal(time.Date(2026, 11, 25, 7, 30, 0, 0, time.UTC)) {
		t.Fatalf("in UTC: %s", got)
	}
	zone = berlin(t)
	if got := s.Next(current); !got.Equal(time.Date(2026, 11, 25, 6, 30, 0, 0, time.UTC)) {
		t.Errorf("after the zone moved to Berlin: %s, want 06:30 UTC", got)
	}
}

func TestAClockTimePassCatchesUpOnlyAMissedSlot(t *testing.T) {
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	every := clockTimeSchedule{at: extension.TimeOfDay{Minute: 7*60 + 30}, zone: func() *time.Location { return time.UTC }, now: clockAt(now)}
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
	if runsAtClockTimes(river.PeriodicInterval(time.Hour)) || !runsAtClockTimes(clockTimeSchedule{}) {
		t.Error("runsAtClockTimes must tell an interval from a clock time")
	}
}
