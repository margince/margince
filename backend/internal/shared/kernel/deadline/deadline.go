// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package deadline answers whether a promised moment has passed.
package deadline

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// Passed reports whether a due moment is in the past — whether the thing is
// LATE.
//
// Strictly past. At the instant a promise falls due it is due, not late: a task
// due at 14:00 is not yet late at 14:00:00, and a reader shown it as overdue in
// that instant would be told they had already failed something they had not.
//
// THIS IS NOT THE SCHEDULER'S QUESTION, and conflating the two is how the two
// readings drifted apart. A scheduler asks "has the moment ARRIVED" — inclusive,
// because a job due now should run now. An overdue display asks "has the moment
// PASSED" — exclusive, because a deadline is not missed until it is behind you.
// Both are correct for their own question and neither answers the other's, so a
// call site that reaches for the wrong one looks right while disagreeing with
// every sibling.
//
// The exclusive reading is also what this product's SQL already asks
// (`due_at < now()`), so a list assembled in Go and a count assembled in the
// database agree about the same row.
//
// A nil due moment is not late. Something with no date promised cannot have
// missed it, and returning true would put every undated item in an overdue list.
func Passed(due *time.Time, now time.Time) bool {
	return due != nil && due.Before(now)
}

// DayPassed reports whether the calendar day of a due moment, read in loc, is
// before today's calendar day in loc. A day-level promise is late only once
// its whole day is over, so the last fractional second of today still counts.
func DayPassed(due, now time.Time, loc *time.Location) bool {
	return due.In(loc).Format(time.DateOnly) < now.In(loc).Format(time.DateOnly)
}

// WallDayOffset is how many calendar days the chosen date of a task lies from
// today in loc: negative once it is behind us, 0 on it. The date is read from
// the wall clock text, not from the instant it resolves to. A reader in another
// zone than the chooser's therefore counts the same day. It reports false when
// the text does not start with a date.
func WallDayOffset(wallClock string, now time.Time, loc *time.Location) (int, bool) {
	if len(wallClock) < len(time.DateOnly) {
		return 0, false
	}
	day, err := time.Parse(time.DateOnly, wallClock[:len(time.DateOnly)])
	if err != nil {
		return 0, false
	}
	noon := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc)
	return elapsed.Days(now, noon, loc), true
}

// TaskPassed is Passed for a task. A task with a chosen date and no time is
// late once that date is behind today in loc, whatever instant the date
// resolves to. Any other task is late by its instant. wallClock is the chosen
// clock's text, empty when the task has none.
func TaskPassed(due *time.Time, hasTime bool, wallClock string, now time.Time, loc *time.Location) bool {
	if due != nil && !hasTime && wallClock != "" {
		if days, ok := WallDayOffset(wallClock, now, loc); ok {
			return days < 0
		}
	}
	return Passed(due, now)
}

// TaskDueOn is the instant a task's lateness and order are judged at. A task
// with a chosen date and no time is due until the start of the next calendar
// day in loc. It is judged at the last instant before that start. Any other
// task, and a nil loc, keep the due instant. storekit.TaskDueOrderSQL spells
// the same rule.
func TaskDueOn(due *time.Time, hasTime bool, wallClock string, loc *time.Location) *time.Time {
	if due == nil || hasTime || wallClock == "" || loc == nil || len(wallClock) < len(time.DateOnly) {
		return due
	}
	day, err := time.Parse(time.DateOnly, wallClock[:len(time.DateOnly)])
	if err != nil {
		return due
	}
	end := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, loc).Add(-time.Nanosecond)
	return &end
}
