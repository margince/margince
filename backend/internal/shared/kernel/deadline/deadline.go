// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package deadline answers whether a promised moment has passed.
package deadline

import "time"

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

// DaysPast is how many WHOLE days a due moment is behind now, and whether it is
// behind at all.
//
// Whole days ELAPSED, not calendar days crossed. The two differ: a promise made
// at 23:00 and judged at 01:00 has crossed a calendar day and elapsed two hours,
// and calling that "a day late" overstates it.
//
// Calendar counting needs a timezone to say where a day begins. The
// installation stores one, but it is not an argument here and this package is
// stdlib-only by tier — so a caller that means calendar days must count them
// where that setting is in scope, as the close-date sweep does.
//
// NOT `kernel/elapsed`, which counts calendar days in UTC and is the right
// answer for ITS question. That one measures silence between two moments and is
// deliberately calendar-based, so its wording can only change when the UTC-day
// cache key it rides does. This one measures how far past a promise we are,
// where elapsed time is what a reader means — 23:00 to 01:00 has crossed a
// calendar day and is not "a day late". Reaching for the wrong one produces a
// count that is off by one for most of every day.
//
// Zero is a real answer: something hours past its date is late by no whole
// days, which is not the same as not being late. Callers read the bool, never
// the count, to decide.
func DaysPast(due *time.Time, now time.Time) (int, bool) {
	if !Passed(due, now) {
		return 0, false
	}
	return int(now.Sub(*due) / (24 * time.Hour)), true
}
