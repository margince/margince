// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Where today ends, and who decides it.
//
// Three lanes are bounded by the end of the day — the agreed work, the promises
// coming due, and what is booked. They are judged against ONE instant, resolved
// once per assembly, because a response that measured two lanes against two
// different days would be reporting two days.

import (
	"context"
	"errors"
	"time"
)

// Zone answers the installation's timezone, which is what decides when "today"
// ends for this feed.
//
// A SEAM rather than a read of its own: this package binds no module and opens
// no transaction, and the installation's timezone is a fact the composition
// root already resolves for the morning brief through identity.TimezoneOf.
type Zone func(ctx context.Context) (*time.Location, error)

// Option adjusts a service after its seams are bound.
//
// Variadic rather than a nineteenth parameter: the constructor already takes
// every lane, and a reader counting arguments to find the one that decides the
// day is a reader who will bind it wrong.
type Option func(*Service)

// WithZone binds the installation timezone the day boundary is measured in.
//
// UNBOUND MEANS UTC, and that is the honest default rather than a hidden one: a
// composition with no installation to ask — every unit test in this package —
// has no local day, and UTC is the only answer that does not invent one. The
// shipped wiring binds it, which is what makes the difference visible.
func WithZone(z Zone) Option {
	return func(s *Service) { s.zone = z }
}

// WithMeetingsAwaitingOutcome binds the lane of meetings that already happened.
//
// An Option rather than a twentieth constructor argument, for the reason above:
// unbound means the feed does not carry the lane, which the response reports as
// absent rather than as a day with nothing left to close off.
func WithMeetingsAwaitingOutcome(m MeetingsAwaitingOutcome) Option {
	return func(s *Service) { s.meetingsAwaitingOutcome = m }
}

// endOfDay is the boundary every due-dated lane stops at, so a promise, a task
// and a meeting falling on the same afternoon are judged against one instant.
//
// THE INSTALLATION'S midnight, not UTC's. Truncating the clock to 24 hours
// answers UTC midnight wherever the installation is, which put a UTC+7 seat's
// "today" through 07:00 tomorrow and cut a UTC-4 seat's short at 20:00 — the
// same convention error the morning brief's local_day exists to avoid.
//
// AddDate over the local date, not Add(24h) over the instant: a day is 23 or 25
// hours where clocks change, and adding a fixed span lands an hour off on those
// two mornings a year — in the direction that drops work the reader is owed.
// ONCE PER ASSEMBLY, and the answer is carried to every lane that needs it. An
// operator moving the installation mid-read would otherwise give one lane
// yesterday's boundary and the next one today's, inside a single response — and
// each resolution is a transaction, so asking per lane also pays three times for
// one fact.
func (s *Service) endOfDay(ctx context.Context, asOf time.Time) (time.Time, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return time.Time{}, err
	}
	return startOfNextDay(asOf.In(loc), loc), nil
}

// startOfDay is the other end of the same day, for a lane that looks BACK.
//
// The forward lanes ask what is still coming and stop at endOfDay; a lane about
// what already happened needs where today began, and "today" has to mean the
// same thing at both ends or a meeting at 08:00 falls between them.
//
// It is startOfNextDay asked about YESTERDAY, rather than a second walk: that
// function's whole subtlety is the zones where local midnight does not exist,
// and a midnight built here directly would be wrong on exactly the mornings
// that comment describes.
func (s *Service) startOfDay(ctx context.Context, asOf time.Time) (time.Time, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return time.Time{}, err
	}
	local := asOf.In(loc)
	return startOfNextDay(local.AddDate(0, 0, -1), loc), nil
}

// location resolves the installation's zone, or UTC when none is bound.
//
// One spelling for both boundaries: they must agree about where the reader is,
// and an operator moving the installation between two resolutions would
// otherwise hand one end of a day to one zone and the other end to another.
func (s *Service) location(ctx context.Context) (*time.Location, error) {
	if s.zone == nil {
		return time.UTC, nil
	}
	resolved, err := s.zone(ctx)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		// A seam that answers "no zone, no error" would otherwise reach
		// time.In with a nil location, which panics — a nil answer is a
		// broken binding, and this says so where the feed can report it.
		return nil, errors.New("attention: the installation timezone resolved to no location")
	}
	return resolved, nil
}

// startOfNextDay is the first instant of the day after local's, in loc.
//
// NOT local midnight constructed directly, because in some zones it does not
// exist. Havana and Santiago spring forward AT midnight, and Go normalises the
// missing 00:00 BACKWARD — to 23:00 on the previous date, an hour before the day
// this bounds has even ended, so the last hour's work would fall off today's
// lane. Beirut springs forward at midnight too and normalises FORWARD, to 01:00,
// which is right by luck rather than by rule.
//
// So the rule is asked directly: the first instant whose local date is
// tomorrow's. Noon is the anchor because no zone shifts its clock at midday, so
// "tomorrow" is never ambiguous to begin with, and the walk back from it stops
// at the transition on the days there is one. At most a few hundred steps, once
// per assembly, and exact on every day of the year rather than on most of them.
func startOfNextDay(local time.Time, loc *time.Location) time.Time {
	noon := time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, loc).AddDate(0, 0, 1)
	year, month, day := noon.Date()
	if midnight := time.Date(year, month, day, 0, 0, 0, 0, loc); sameDate(midnight, noon) {
		return midnight
	}
	first := noon
	for {
		earlier := first.Add(-time.Minute)
		if !sameDate(earlier, noon) {
			return first
		}
		first = earlier
	}
}

// sameDate compares two instants by the LOCAL calendar date they fall on.
func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
