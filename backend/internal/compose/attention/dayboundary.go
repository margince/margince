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
	loc := time.UTC
	if s.zone != nil {
		resolved, err := s.zone(ctx)
		if err != nil {
			return time.Time{}, err
		}
		if resolved == nil {
			// A seam that answers "no zone, no error" would otherwise reach
			// time.In with a nil location, which panics — a nil answer is a
			// broken binding, and this says so where the feed can report it.
			return time.Time{}, errors.New("attention: the installation timezone resolved to no location")
		}
		loc = resolved
	}
	local := asOf.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1), nil
}
