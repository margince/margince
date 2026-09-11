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

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
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

// WithNoticeCases binds the lane of disclosure duties nobody has discharged.
//
// An Option for the reason above, and one that matters more here than for most
// lanes: an installation whose feed does not read the notice queue must report
// the lane ABSENT rather than empty. A duty queue rendered as "nothing owed"
// when nobody looked is the exact failure this lane exists to end.
func WithNoticeCases(n NoticeCases) Option {
	return func(s *Service) { s.noticeCases = n }
}

// endOfDay is the boundary every due-dated lane stops at, so a promise, a task
// and a meeting falling on the same afternoon are judged against one instant.
//
// THE INSTALLATION'S midnight, not UTC's — storekit.StartOfNextDay derives it
// in the installation's zone, exact on the two mornings a year the clocks move
// where a truncated 24 hours would land an hour off. The same primitive backs
// every calendar-day surface the product derives.
//
// ONCE PER ASSEMBLY, and the answer is carried to every lane that needs it. An
// operator moving the installation mid-read would otherwise give one lane
// yesterday's boundary and the next one today's, inside a single response — and
// each resolution is a transaction, so asking per lane also pays three times for
// one fact.
// It returns the ZONE it resolved beside the boundary, because the grouping
// below needs the same one: asked again per lane, an operator moving the
// installation mid-read would give one lane yesterday's zone and the next
// today's, inside a single response.
func (s *Service) endOfDay(ctx context.Context, asOf time.Time) (time.Time, *time.Location, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return time.Time{}, nil, err
	}
	return storekit.StartOfNextDay(asOf, loc), loc, nil
}

// startOfDay is the other end of the same day, for a lane that looks BACK.
//
// The forward lanes ask what is still coming and stop at endOfDay; a lane about
// what already happened needs where today began, and "today" has to mean the
// same thing at both ends or a meeting at 08:00 falls between them. Both ends
// come from the storekit primitive, so a backward lane and a forward one
// measure the day by one rule.
func (s *Service) startOfDay(ctx context.Context, asOf time.Time) (time.Time, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return time.Time{}, err
	}
	return storekit.StartOfDay(asOf, loc), nil
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

// The five runs a dated row can fall into. Named rather than spelled at each
// arm: the same words are the contract's enum and the client's copy keys, and a
// typo in one of them renders a heading nobody has translated.
const (
	dueGroupOverdue  = "overdue"
	dueGroupToday    = "today"
	dueGroupTomorrow = "tomorrow"
	dueGroupThisWeek = "this_week"
	dueGroupLater    = "later"
)

// upcomingWeekDays is how far past today "this week" reaches. A rolling seven
// days rather than a calendar week: on a Sunday a calendar week would name the
// day itself, and a reader asking what is coming means the next seven days
// whichever day they ask on.
const upcomingWeekDays = 7

// dueGroup names which run of the page a dated row belongs to, so the client
// can head "Due tomorrow" and "Due this week" without deciding the boundary
// itself.
//
// The server decides it for the reason every other boundary on this page is the
// server's: the day's end depends on the installation's zone, and a browser
// computing it from its own clock puts a task in a different group from the
// count sitting above it.
//
// asOf and until are the same two instants the whole assembly runs on: they are
// resolved once in assembleDay and handed down, rather than re-derived per lane.
func dueGroup(deadlineAt, asOf, until time.Time, loc *time.Location) string {
	// Lateness is deadline.Passed's decision, never a comparison spelled here:
	// a list, a card and an agent tool once disagreed about a promise due at
	// this very instant because each made the call itself.
	if deadline.Passed(&deadlineAt, asOf) {
		return dueGroupOverdue
	}
	// The rest compare against DAY BOUNDARIES rather than against the clock,
	// which is a different question and one this file owns: where today ends,
	// where tomorrow ends, and how far "this week" reaches.
	tomorrowEnds := storekit.StartOfNextDay(until, loc)
	weekEnds := until.AddDate(0, 0, upcomingWeekDays)
	switch {
	case deadlineAt.Before(until):
		return dueGroupToday
	case deadlineAt.Before(tomorrowEnds):
		return dueGroupTomorrow
	case deadlineAt.Before(weekEnds):
		return dueGroupThisWeek
	default:
		return dueGroupLater
	}
}
