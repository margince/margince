// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package occurredday is where a date-only activity's date and its stored
// instant meet. Every writer stores a day entry through Anchor, and every reader
// takes its date back through Date, both in the record zone.
//
// In kernel because the writers and readers sit in sibling modules (activities,
// capture, search) and in compose, and no module imports a sibling.
package occurredday

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// Anchor is the instant a day entry naming date is stored at: noon of that date
// in zone. Only date's own year, month and day are read, so the caller decides
// which offset names the date. No zone moves its clock at noon.
func Anchor(date time.Time, zone *time.Location) time.Time {
	year, month, day := date.Date()
	return time.Date(year, month, day, 12, 0, 0, 0, zone).UTC()
}

// Date is the date a stored day entry names, read in zone, as that date's UTC
// midnight (the form a `date` column reads back).
func Date(at time.Time, zone *time.Location) time.Time {
	return elapsed.CalendarDay(at, zone)
}

// When is an activity's time as text for a reader not told the precision. It is
// a day entry's date, and the RFC 3339 instant otherwise.
func When(at time.Time, dateOnly bool, zone *time.Location) string {
	if dateOnly {
		return Date(at, zone).Format(time.DateOnly)
	}
	return at.UTC().Format(time.RFC3339)
}

// Ended reports whether the date a day entry names is over at asOf. A day entry
// belongs to its whole date, so it is neither ahead nor past while that runs.
func Ended(at, asOf time.Time, zone *time.Location) bool {
	return Date(asOf, zone).After(Date(at, zone))
}
