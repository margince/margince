// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package occurredday is where a date-only activity's date and its stored
// instant meet. A day entry is stored at noon UTC of its date, so its date is
// its UTC date and no zone setting moves it. A zone answers only which date
// is today.
//
// A day meeting has taken place once its date is over in the installation zone,
// as the outcome lane and meeting prep read it. An instant meeting has once it
// starts. Every other day entry has happened from the start of its date. A day
// entry falls in a period of local dates when its own date is one of them.
//
// In kernel because the writers and readers sit in sibling modules (activities,
// capture, search, deals) and in compose, and no module imports a sibling.
package occurredday

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// day is the stored and wire precision of an entry that names only a date.
const day = "day"

// meetingKind is the activity kind whose day entries last their whole date.
const meetingKind = "meeting"

// Moment is an activity's stored time and whether it names only a date.
type Moment struct {
	At       time.Time
	DateOnly bool
}

// IsDay says a stored or wire precision names only a date.
func IsDay[P ~string](precision P) bool {
	return precision == day
}

// IsStatedDay is IsDay for a precision the wire may leave out, which it does
// only beside an absent time.
func IsStatedDay[P ~string](precision *P) bool {
	return precision != nil && IsDay(*precision)
}

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
func DateIn(m Moment, zone *time.Location) time.Time {
	if m.DateOnly {
		return Date(m.At)
	}
	return elapsed.CalendarDay(m.At, zone)
}

// Begins is when the date a day entry names begins in zone: the moment that
// stands for the day where an hour is wanted.
func Begins(at time.Time, zone *time.Location) time.Time {
	year, month, day := Date(at).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, zone)
}

// When is an activity's time as text for a reader not told the precision. It is
// a day entry's date, and the RFC 3339 instant otherwise.
func When(m Moment) string {
	if m.DateOnly {
		return Date(m.At).Format(time.DateOnly)
	}
	return m.At.UTC().Format(time.RFC3339)
}

// Ended reports whether the date a day entry names is over at asOf, which is
// when today's date in zone is a later one. EndSQL is the same rule in SQL.
func Ended(at, asOf time.Time, zone *time.Location) bool {
	return at.Before(AnchorIn(asOf, zone))
}

// Begun reports whether an activity has happened at asOf: an instant once
// asOf reaches it, a day entry once its date has begun in zone. BegunSQL is the
// same rule in SQL.
func Begun(m Moment, asOf time.Time, zone *time.Location) bool {
	if m.DateOnly {
		return !m.At.After(AnchorIn(asOf, zone))
	}
	return !m.At.After(asOf)
}

// MeetingTookPlace reports whether a meeting has taken place at asOf. An
// instant meeting has once it starts. A day meeting has once its date is over in
// zone (Ended), as the outcome lane and meeting prep read it.
func MeetingTookPlace(m Moment, asOf time.Time, zone *time.Location) bool {
	if m.DateOnly {
		return Ended(m.At, asOf, zone)
	}
	return !m.At.After(asOf)
}

// Happened reports whether an activity of kind has happened at asOf. A meeting
// has once it took place (MeetingTookPlace), any other kind once it has begun
// (Begun). HappenedSQL is the same rule in SQL.
func Happened(kind string, m Moment, asOf time.Time, zone *time.Location) bool {
	if kind == meetingKind {
		return MeetingTookPlace(m, asOf, zone)
	}
	return Begun(m, asOf, zone)
}

// DaysSince counts the calendar days from the date an activity names to
// today's date in zone. A day entry dated today is zero days old.
func DaysSince(m Moment, asOf time.Time, zone *time.Location) int {
	year, month, day := DateIn(m, zone).Date()
	return elapsed.Days(time.Date(year, month, day, 12, 0, 0, 0, zone), asOf, zone)
}

// DaysBetween counts the calendar days from the date one activity names to the
// date a later one names, each read by its own precision.
func DaysBetween(from, to Moment, zone *time.Location) int {
	later := to.At
	if to.DateOnly {
		later = Begins(later, zone)
	}
	return DaysSince(from, later, zone)
}

// BegunSQL is Begun for the activity under alias at asOf, both SQL the caller
// renders. A day entry stored at or before asOf is at or before today's anchor
// too. So one bound on occurred_at holds both arms and keeps a range scan.
func BegunSQL(alias, asOf string) string {
	return fmt.Sprintf(`(%[1]s.occurred_at <= greatest(%[2]s, %[3]s) AND (%[1]s.occurred_precision = 'day' OR %[1]s.occurred_at <= %[2]s))`,
		alias, asOf, todayAnchorSQL(asOf))
}

// MeetingTookPlaceSQL is MeetingTookPlace for the meeting under alias at asOf,
// both SQL the caller renders; AheadSQL is its negation. A day row's
// occurred_at is an anchor, so "its date is over" is an anchor before today's.
func MeetingTookPlaceSQL(alias, asOf string) string {
	return fmt.Sprintf(`(%[1]s.occurred_at <= greatest(%[2]s, %[3]s) AND (%[1]s.occurred_precision = 'day' AND %[1]s.occurred_at < %[3]s OR %[1]s.occurred_precision = 'instant' AND %[1]s.occurred_at <= %[2]s))`,
		alias, asOf, todayAnchorSQL(asOf))
}

// HappenedSQL is Happened for the activity under alias at asOf.
func HappenedSQL(alias, asOf string) string {
	return fmt.Sprintf(`((%[1]s.kind = '%[2]s' AND %[3]s) OR (%[1]s.kind <> '%[2]s' AND %[4]s))`,
		alias, meetingKind, MeetingTookPlaceSQL(alias, asOf), BegunSQL(alias, asOf))
}

// todayAnchorSQL is AnchorIn(asOf, installation zone) in SQL.
func todayAnchorSQL(asOf string) string {
	return `((date_trunc('day', CAST(` + asOf + ` AS timestamptz) AT TIME ZONE ` + installationZoneSQL +
		`) + interval '12 hours') AT TIME ZONE 'UTC')`
}

// EndSQL is when the activity under alias ends. An instant ends at its start
// plus its duration. A day entry ends with its date in the installation zone,
// Ended's rule. alias is SQL the caller renders, never request text.
func EndSQL(alias string) string {
	return fmt.Sprintf(`(CASE WHEN %[1]s.occurred_precision = 'day'
		THEN %[2]s
		ELSE %[1]s.occurred_at + make_interval(secs => coalesce(%[1]s.duration_seconds, 0)) END)`, alias, dateEndSQL(alias))
}

// ProvenSQL is the first moment the activity under alias is known to have
// happened. An instant is at its time, a day entry once its date ends (Ended).
// A date proves nothing about its hours, so a deadline in hours reads this.
func ProvenSQL(alias string) string {
	return fmt.Sprintf(`(CASE WHEN %[1]s.occurred_precision = 'day' THEN %[2]s ELSE %[1]s.occurred_at END)`,
		alias, dateEndSQL(alias))
}

// LatestPrecisionSQL is the precision of the newest activity under alias that
// filter admits, instant when an instant ties a day entry's noon. It is an
// aggregate of two maxima, so no group is sorted, and null when filter admits no row.
// An empty alias reads bare columns, and an empty filter admits every row.
func LatestPrecisionSQL(alias, filter string) string {
	column := func(name string) string {
		if alias == "" {
			return name
		}
		return alias + "." + name
	}
	at, instant := column("occurred_at"), column("occurred_precision")+" = 'instant'"
	admitted, newestInstant := "", " FILTER (WHERE "+instant+")"
	if filter != "" {
		admitted = " FILTER (WHERE " + filter + ")"
		newestInstant = " FILTER (WHERE " + instant + " AND (" + filter + "))"
	}
	return fmt.Sprintf(`(CASE WHEN max(%[1]s)%[2]s = max(%[1]s)%[3]s THEN 'instant'
		WHEN max(%[1]s)%[3]s IS NOT NULL THEN 'day' END)`, at, newestInstant, admitted)
}

// dateEndSQL is when the date the day entry under alias names ends in the
// installation zone.
func dateEndSQL(alias string) string {
	return `(date_trunc('day', ` + alias + `.occurred_at AT TIME ZONE 'UTC') + interval '1 day') AT TIME ZONE ` + installationZoneSQL
}

// StartSQL is when the activity under alias starts: an instant at its time,
// a day entry when its date begins in the installation zone (Begins). It is
// the order a calendar reads, and where "after the meeting" begins.
func StartSQL(alias string) string {
	return StartAtSQL(alias+".occurred_at", alias+".occurred_precision")
}

// IsAnchorSQL is "at could be a date's Anchor": noon UTC to the microsecond. A day entry
// is always stored there, so any other instant is a time of day.
func IsAnchorSQL(at string) string {
	return `((` + at + ` AT TIME ZONE 'UTC')::time = TIME '12:00:00')`
}

// StartAtSQL is StartSQL for an activity time held elsewhere than its own
// row: at is the instant stored the way occurred_at is, precision its kind.
func StartAtSQL(at, precision string) string {
	return fmt.Sprintf(`(CASE WHEN %[2]s = 'day'
		THEN date_trunc('day', %[1]s AT TIME ZONE 'UTC') AT TIME ZONE `+installationZoneSQL+`
		ELSE %[1]s END)`, at, precision)
}

// WithinSQL is "the activity under alias falls in the period [from, to)", both
// bounds SQL the caller renders. An instant falls in it by its time. A day entry
// falls in it where its date begins in the installation zone (StartSQL). So a
// period of whole local dates holds the day entries those dates name, no more.
func WithinSQL(alias, from, to string) string {
	return WithinAtSQL(alias+".occurred_at", alias+".occurred_precision", from, to)
}

// WithinAtSQL is WithinSQL for an activity time held elsewhere (StartAtSQL).
func WithinAtSQL(at, precision, from, to string) string {
	return "(" + fromAtSQL(at, precision, from) + " AND " + beforeAtSQL(at, precision, to) + ")"
}

// FromSQL is WithinSQL's lower edge alone. A date begins at or before its
// anchor, so `occurred_at >= from` holds every row that passes and keeps a
// range scan.
func FromSQL(alias, from string) string {
	return fromAtSQL(alias+".occurred_at", alias+".occurred_precision", from)
}

func fromAtSQL(at, precision, from string) string {
	return fmt.Sprintf(`(%[1]s >= %[3]s AND (%[2]s = 'instant' OR %[4]s >= %[3]s))`,
		at, precision, from, StartAtSQL(at, precision))
}

// BeforeSQL is WithinSQL's upper edge alone. A date begins at most 26 hours
// before its anchor (UTC+14), the bound that keeps a range scan.
func BeforeSQL(alias, to string) string {
	return beforeAtSQL(alias+".occurred_at", alias+".occurred_precision", to)
}

func beforeAtSQL(at, precision, to string) string {
	return fmt.Sprintf(`(%[1]s < CAST(%[3]s AS timestamptz) + interval '26 hours'
		AND (%[2]s = 'instant' AND %[1]s < %[3]s OR %[2]s = 'day' AND %[4]s < %[3]s))`,
		at, precision, to, StartAtSQL(at, precision))
}

// DatedFromSQL is "the activity under alias falls on or after the date in the
// date column date". The date is read at its UTC midnight, the instant a date
// binds to (storekit.AsDate). So a day entry compares by its own date.
func DatedFromSQL(alias, date string) string {
	return fmt.Sprintf(`(%[2]s::timestamp AT TIME ZONE 'UTC' <= %[1]s.occurred_at)`, alias, date)
}

// DatesSQL is "the day entry under alias names a date in [from, before)", both
// bounds anchors (Anchor) the caller renders. Anchors order as their dates do.
func DatesSQL(alias, from, before string) string {
	return fmt.Sprintf(`(%[1]s.occurred_precision = 'day' AND %[1]s.occurred_at >= %[2]s AND %[1]s.occurred_at < %[3]s)`,
		alias, from, before)
}

// AheadSQL is "the meeting under alias is still to come" at now, the negation
// of MeetingTookPlaceSQL. That is an instant that starts later, or a day entry
// whose date has not ended (Ended). A date ends less than 25 hours after its
// anchor, a bound that keeps a range scan.
func AheadSQL(alias, now string) string {
	return fmt.Sprintf(`((%[1]s.occurred_precision = 'instant' AND %[1]s.occurred_at > %[2]s)
		OR (%[1]s.occurred_precision = 'day' AND %[1]s.occurred_at > CAST(%[2]s AS timestamptz) - interval '25 hours'
		    AND %[3]s > %[2]s))`, alias, now, EndSQL(alias))
}

// installationZoneSQL is the installation zone as SQL reads it: the row of
// identity.Timezone, else that entry's default. A subquery, so a statement
// judging many meetings reads the zone once and binds nothing.
const installationZoneSQL = `COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')`
