// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package elapsed is the one spelling of "how many days of silence".
//
// Held by: TestOnlyElapsedCountsDaysOfSilence (backend/elapsedonespelling_test.go)
//
// Two surfaces answered this and they disagreed on screen. The deal card's
// move said "They wrote 96 days ago"; the coverage chip beside it said "95
// days", about the same mail, on the same page, in the same second. One
// counted CALENDAR days and the other counted elapsed hours divided by 24, so
// the two drifted apart for most of every day and agreed only around midnight.
//
// The calendar count is the correct one, and not as a matter of taste. The
// deal card is cached on the UTC day: counting elapsed hours would flip
// "today" to "yesterday" twenty-four hours after the contact — mid-afternoon,
// say — while the cache key waits for midnight, and the card would spend the
// gap saying something the records do not support with nothing to notice.
// Counting the same way the key does means the wording can only change when
// the key does.
//
// Stdlib-only and in kernel because the two callers are in different compose
// subpackages, and a module never imports a sibling (ADR-0054 §3). Putting it
// in either one would have left the other reaching across that line.
package elapsed

import "time"

// hoursPerDay is the divisor a duration-based count uses. Named because the
// bare 24 in `d.Hours()/24` is exactly what made the old count look correct.
const hoursPerDay = 24

// Days counts whole days between two moments BY THE CALENDAR, in UTC.
//
// Not by elapsed duration: 23:00 Monday to 01:00 Tuesday is one day here and
// zero by the clock, and a reader looking at two dates counts it the way this
// does. Negative when `to` precedes `from`, which callers reading a future
// timestamp rely on to tell a plan from an event.
func Days(from, to time.Time) int {
	fromDay := from.UTC().Truncate(hoursPerDay * time.Hour)
	toDay := to.UTC().Truncate(hoursPerDay * time.Hour)
	return int(toDay.Sub(fromDay).Hours() / hoursPerDay)
}
