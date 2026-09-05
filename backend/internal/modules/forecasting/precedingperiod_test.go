// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// The series a historical median is read over walks backwards one window at a
// time. Every case below is a way that walk can land off the calendar.

import (
	"testing"
	"time"
)

// localMidnight is how ResolveWeek wants its Monday: the zone's own midnight,
// not an instant inside the day.
func localMidnight(year int, month time.Month, d int, zone *time.Location) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, zone)
}

func TestTheQuarterBeforeAQuarterIsTheWholeQuarterBefore(t *testing.T) {
	t.Parallel()
	zone := berlin(t)
	q2, err := ResolvePeriod(
		PeriodQuarter, time.Date(2026, time.May, 15, 12, 0, 0, 0, zone), 1, zone)
	if err != nil {
		t.Fatalf("resolving Q2: %v", err)
	}

	previous, ok := PrecedingPeriod(q2)
	if !ok {
		t.Fatal("a resolved quarter has a predecessor and this reported none")
	}
	if got := previous.StartDate.Format("2006-01-02"); got != "2026-01-01" {
		t.Errorf("the quarter before Q2 starts %s, want 2026-01-01", got)
	}
	if got := previous.EndDate.Format("2006-01-02"); got != "2026-03-31" {
		t.Errorf("the quarter before Q2 ends %s, want 2026-03-31", got)
	}
}

// A February step is the case a duration-based walk gets wrong: 31 days back
// from 1 March is 29 January, which would read a month the previous window
// already covered and double-count its deals in the series.
func TestTheMonthBeforeMarchIsFebruaryAndNotThirtyOneDays(t *testing.T) {
	t.Parallel()
	zone := berlin(t)
	march, err := ResolvePeriod(
		PeriodMonth, time.Date(2026, time.March, 10, 12, 0, 0, 0, zone), 1, zone)
	if err != nil {
		t.Fatalf("resolving March: %v", err)
	}

	previous, ok := PrecedingPeriod(march)
	if !ok {
		t.Fatal("March has a predecessor and this reported none")
	}
	if got := previous.StartDate.Format("2006-01-02"); got != "2026-02-01" {
		t.Errorf("the month before March starts %s, want 2026-02-01", got)
	}
	if got := previous.EndDate.Format("2006-01-02"); got != "2026-02-28" {
		t.Errorf("the month before March ends %s, want 2026-02-28", got)
	}
}

// The window whose midnights move. Stepping back 7*24h across the spring change
// lands at 23:00 the previous day, and every day bound after it is an hour out.
func TestTheWeekBeforeADaylightChangeKeepsItsLocalMidnights(t *testing.T) {
	t.Parallel()
	zone := berlin(t)
	// Germany's clocks go forward on Sunday 29 March 2026, inside this week.
	week, err := ResolveWeek(localMidnight(2026, time.March, 30, zone), zone)
	if err != nil {
		t.Fatalf("resolving the week: %v", err)
	}

	previous, ok := PrecedingPeriod(week)
	if !ok {
		t.Fatal("a resolved week has a predecessor and this reported none")
	}
	if got := previous.StartDate.Format("2006-01-02"); got != "2026-03-23" {
		t.Errorf("the week before starts %s, want 2026-03-23", got)
	}
	if got := previous.EndDate.Format("2006-01-02"); got != "2026-03-29" {
		t.Errorf("the week before ends %s, want 2026-03-29", got)
	}
	if hour := previous.Start.In(zone).Hour(); hour != 0 {
		t.Errorf("the previous week opens at %02d:00 local, want midnight — a window "+
			"stepped by a fixed duration drifts across a daylight change", hour)
	}
}

// The zone whose daylight change falls BETWEEN two weekly midnights rather than
// inside the week. Berlin's spring change sits inside its week, so the week is
// still 168 hours from midnight to midnight and a duration-derived day count
// survives it. Cairo's does not: 20-26 April 2026 is 143 hours, which divides
// to six days and steps the predecessor onto a Tuesday.
func TestAWeekWhoseZoneChangesOffsetBetweenItsMidnightsStillStepsAWholeWeek(t *testing.T) {
	t.Parallel()
	zone, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		t.Fatalf("loading Africa/Cairo: %v", err)
	}
	week, err := ResolveWeek(localMidnight(2026, time.April, 20, zone), zone)
	if err != nil {
		t.Fatalf("resolving the week: %v", err)
	}

	previous, ok := PrecedingPeriod(week)
	if !ok {
		t.Fatal("a resolved week has a predecessor and this reported none")
	}
	if got := previous.StartDate.Weekday(); got != time.Monday {
		t.Errorf("the previous week starts on a %s, want Monday — a day count divided "+
			"out of elapsed hours loses a day when the offset moves between two "+
			"midnights, and every window behind it is then a day out", got)
	}
	if got := previous.StartDate.Format("2006-01-02"); got != "2026-04-13" {
		t.Errorf("the week before 20 April starts %s, want 2026-04-13", got)
	}
	if got := previous.EndDate.Format("2006-01-02"); got != "2026-04-19" {
		t.Errorf("the week before 20 April ends %s, want 2026-04-19", got)
	}
}

// Four steps back is what a median needs, and each must be a distinct window:
// a walk that repeats one would take a median of the same quarter four times.
func TestWalkingBackFourQuartersGivesFourDistinctWindows(t *testing.T) {
	t.Parallel()
	zone := berlin(t)
	window, err := ResolvePeriod(
		PeriodQuarter, time.Date(2026, time.May, 15, 12, 0, 0, 0, zone), 1, zone)
	if err != nil {
		t.Fatalf("resolving Q2: %v", err)
	}

	seen := map[string]bool{}
	for i := range ComparablePeriodsNeeded() {
		previous, ok := PrecedingPeriod(window)
		if !ok {
			t.Fatalf("step %d reported no predecessor", i)
		}
		key := previous.StartDate.Format("2006-01-02")
		if seen[key] {
			t.Fatalf("step %d returned %s again — a repeated window makes a median of "+
				"one period wearing four votes", i, key)
		}
		seen[key] = true
		if !previous.EndDate.Before(window.StartDate) {
			t.Errorf("step %d ends %s, which is not before the window it precedes (%s)",
				i, previous.EndDate.Format("2006-01-02"), window.StartDate.Format("2006-01-02"))
		}
		window = previous
	}
	if len(seen) != ComparablePeriodsNeeded() {
		t.Errorf("walked back %d distinct windows, want %d",
			len(seen), ComparablePeriodsNeeded())
	}
}

// A zero Period is what an unresolved window looks like, and it must not
// produce a window covering the year 1.
func TestAnUnresolvedPeriodHasNoPredecessor(t *testing.T) {
	t.Parallel()
	if _, ok := PrecedingPeriod(Period{}); ok {
		t.Error("a zero period reported a predecessor — a window with no dates would " +
			"read every deal ever closed as one comparable period")
	}
}
