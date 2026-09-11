// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"time"
)

// LoadZone resolves an IANA zone name to a *time.Location, wrapping the failure
// with the name so an operator sees which zone did not resolve.
//
// The one place the installation's zone STRING becomes a location: WorkspaceDay
// and the boundary helpers all take the *time.Location it returns, so every
// module that derives a day loads the zone the same way and reports an
// unresolvable one in the same words.
func LoadZone(name string) (*time.Location, error) {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("the installation's timezone %q does not resolve: %w", name, err)
	}
	return loc, nil
}

// Where a calendar day comes from, spelled once.
//
// An instant is zone-independent; the day it falls on is not. Truncating the
// clock to 24 hours answers UTC's midnight wherever the installation is, which
// puts a seat east of UTC on yesterday for its whole local morning and a seat
// west of UTC on tomorrow for its whole local evening. Every "what day is it"
// on a business surface — the day a rate takes effect, the day a contract is
// judged against, the day a close is scored from — derives here instead, in the
// installation's own zone, so a fourth caller cannot re-break the invariant by
// reaching for Truncate again.

// WorkspaceDay is the calendar day the instant falls on in loc, as a DATE:
// the local year-month-day re-anchored at UTC midnight.
//
// A date, not an instant, because that is what a `date` column and a `::date`
// bind read back unchanged. A value carrying the local wall time would bind as
// a different day west of UTC — the exact off-by-one this helper exists to end.
func WorkspaceDay(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// AsDate reduces a value to the calendar date it ALREADY names — its UTC
// year-month-day at UTC midnight — projecting through no zone.
//
// The counterpart to WorkspaceDay, and not interchangeable with it. WorkspaceDay
// answers "what day is this INSTANT, locally"; AsDate answers "normalise this
// DATE the caller already chose". Passing a chosen date through WorkspaceDay
// would slip it a day west of UTC — the off-by-one a calendar date must never
// take. Use AsDate for an operator-selected effective date; use WorkspaceDay for
// the clock.
func AsDate(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

// StartOfNextDay is the first instant of the day after t's, in loc.
//
// NOT local midnight constructed directly, because in some zones it does not
// exist. Havana and Santiago spring forward AT midnight, and Go normalises the
// missing 00:00 BACKWARD — to 23:00 on the previous date, an hour before the day
// this bounds has even begun. So the rule is asked directly: the first instant
// whose local date is tomorrow's. Noon is the anchor because no zone shifts its
// clock at midday, so "tomorrow" is never ambiguous, and the walk back from it
// stops at the transition on the days there is one.
func StartOfNextDay(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	noon := time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, loc).AddDate(0, 0, 1)
	year, month, day := noon.Date()
	if midnight := time.Date(year, month, day, 0, 0, 0, 0, loc); sameLocalDate(midnight, noon) {
		return midnight
	}
	first := noon
	for {
		earlier := first.Add(-time.Minute)
		if !sameLocalDate(earlier, noon) {
			return first
		}
		first = earlier
	}
}

// StartOfDay is the first instant of t's OWN calendar day, in loc.
//
// It is StartOfNextDay asked about yesterday rather than a second midnight walk:
// that function's whole subtlety is the zones where local midnight does not
// exist, and a midnight built here directly would be wrong on exactly those
// mornings.
func StartOfDay(t time.Time, loc *time.Location) time.Time {
	return StartOfNextDay(t.In(loc).AddDate(0, 0, -1), loc)
}

// sameLocalDate compares two instants by the LOCAL calendar date they fall on.
func sameLocalDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
