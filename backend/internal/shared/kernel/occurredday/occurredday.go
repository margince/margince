// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package occurredday is where a date-only activity's date and its stored
// instant meet. A day entry is stored at noon UTC of its date, so its date is
// its UTC date and no zone setting moves it. A zone answers only which date
// is today.
//
// In kernel because the writers and readers sit in sibling modules (activities,
// capture, search, deals) and in compose, and no module imports a sibling.
package occurredday

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// Anchor is the instant a day entry naming date is stored at: noon UTC of that
// date. Only date's own year, month and day are read, so the writer's offset
// names the date.
func Anchor(date time.Time) time.Time {
	year, month, day := date.Date()
	return time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
}

// AnchorIn is the anchor of the date at falls on in zone: today's date for a
// clock, the real local day for a stored instant.
func AnchorIn(at time.Time, zone *time.Location) time.Time {
	return Anchor(elapsed.CalendarDay(at, zone))
}

// Date is the date a stored day entry names, as that date's UTC midnight (the
// form a `date` column reads back).
func Date(at time.Time) time.Time {
	return elapsed.CalendarDay(at, time.UTC)
}

// DateIn is the date an activity names: a day entry's own date, and the date
// an instant falls on in zone.
func DateIn(at time.Time, dateOnly bool, zone *time.Location) time.Time {
	if dateOnly {
		return Date(at)
	}
	return elapsed.CalendarDay(at, zone)
}

// Begins is when the date a day entry names begins in zone: the moment that
// stands for the day where an hour is wanted.
func Begins(at time.Time, zone *time.Location) time.Time {
	year, month, day := Date(at).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, zone)
}

// When is an activity's time as text for a reader not told the precision. It is
// a day entry's date, and the RFC 3339 instant otherwise.
func When(at time.Time, dateOnly bool) string {
	if dateOnly {
		return Date(at).Format(time.DateOnly)
	}
	return at.UTC().Format(time.RFC3339)
}

// Ended reports whether the date a day entry names is over at asOf, which is
// when today's date in zone is a later one. EndSQL is the same rule in SQL.
func Ended(at, asOf time.Time, zone *time.Location) bool {
	return at.Before(AnchorIn(asOf, zone))
}

// EndSQL is when the activity under alias ends. An instant ends at its start
// plus its duration. A day entry ends with its date in the installation zone,
// Ended's rule. alias is SQL the caller renders, never request text.
func EndSQL(alias string) string {
	return fmt.Sprintf(`(CASE WHEN %[1]s.occurred_precision = 'day'
		THEN (date_trunc('day', %[1]s.occurred_at AT TIME ZONE 'UTC') + interval '1 day') AT TIME ZONE `+installationZoneSQL+`
		ELSE %[1]s.occurred_at + make_interval(secs => coalesce(%[1]s.duration_seconds, 0)) END)`, alias)
}

// installationZoneSQL is the installation zone as SQL reads it: the row of
// identity.Timezone, else that entry's default. A subquery, so a statement
// judging many meetings reads the zone once and binds nothing.
const installationZoneSQL = `COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')`
