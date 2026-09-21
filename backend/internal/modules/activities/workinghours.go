// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// When one host is bookable.
//
// A pair of Go constants before this — 9 and 17, read on a UTC instant, with no
// way for anyone to change either. "9am" therefore meant 4pm in Ho Chi Minh
// City, and a Monday morning in Saigon was still Sunday to the scheduler.
//
// Which hours a host keeps is a fact about that CONTACT, and identity owns it.
// This module may not import a sibling, so what arrives here is a value and a
// resolver compose injects. docs/explanation/scheduling.md is the design.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WorkingHours is when one host is bookable, as minutes past midnight on the
// days they work, read on their own zone.
//
// A value type rather than a reach into identity: this module may not import a
// sibling, and the hours are a fact about a contact that identity owns. compose
// injects the resolver that answers it.
type WorkingHours struct {
	StartMinute int
	// EndMinute is exclusive, and may be 24*60: a working day that runs to
	// midnight is one somebody keeps, and 23:59 is a different answer.
	EndMinute int
	// Days are ISO-8601 weekday numbers, 1 = Monday.
	Days []int
	// Location is the zone the two minutes are read on, never nil.
	Location *time.Location
}

// WorkingHoursResolver answers which hours a host keeps.
//
// Nil in a deployment that has not wired it, and then every host is bookable
// 09:00–17:00 Monday to Friday in UTC — the behaviour this package had for
// everybody before the setting existed. It degrades rather than refusing
// because the public booking page reaches this path: a wiring gap must not
// become a customer-facing error about somebody's settings.
type WorkingHoursResolver func(ctx context.Context, host ids.UserID) (WorkingHours, error)

// fallbackWorkingHours is what an unwired store answers with. Named, because it
// is a decision: treating unset as no constraint lets a customer book somebody
// at 3am.
func fallbackWorkingHours() WorkingHours {
	return WorkingHours{
		StartMinute: 9 * 60, EndMinute: 17 * 60,
		Days: []int{1, 2, 3, 4, 5}, Location: time.UTC,
	}
}

// works reports whether a local instant falls on a day this host works.
func (h WorkingHours) works(at time.Time) bool {
	weekday := int(at.Weekday())
	if weekday == 0 {
		// Go counts Sunday as 0; ISO-8601, which the setting speaks, counts it
		// as 7. Converting here rather than storing Go's numbering keeps the
		// wire and the database reading as the standard says.
		weekday = 7
	}
	for _, day := range h.Days {
		if day == weekday {
			return true
		}
	}
	return false
}

// covers reports whether a candidate slot lies inside one working day.
//
// Both ends are read in the host's zone, and the END is compared as a minute
// past ITS OWN midnight only when it lands on the same local day: a slot that
// runs past midnight leaves the working day whatever the clock says, and
// comparing its end against the following day's minutes would admit it.
func (h WorkingHours) covers(start, end time.Time) bool {
	localStart, localEnd := start.In(h.Location), end.In(h.Location)
	if !h.works(localStart) {
		return false
	}
	startMinute := localStart.Hour()*60 + localStart.Minute()
	endMinute := localEnd.Hour()*60 + localEnd.Minute()
	if localEnd.YearDay() != localStart.YearDay() || localEnd.Year() != localStart.Year() {
		// Midnight exactly is the day's own end rather than the next day's
		// start, which is what makes a 23:30–24:00 slot bookable for somebody
		// who works until midnight.
		if endMinute != 0 {
			return false
		}
		endMinute = 24 * 60
	}
	return startMinute >= h.StartMinute && endMinute <= h.EndMinute
}
