// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"encoding/json"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// Occurred is when an activity happened, the brief's own spelling of
// occurredday.Moment so it can state itself to a prompt.
type Occurred occurredday.Moment

// MarshalJSON is the time as a prompt states it, so the model never reads a day
// entry's stored noon as a time somebody named.
func (o Occurred) MarshalJSON() ([]byte, error) {
	return json.Marshal(occurredday.When(o.At, o.DateOnly))
}

// date is the calendar date o names: a day entry's own date, an instant's date
// in loc.
func (o Occurred) date(loc *time.Location) time.Time {
	return occurredday.DateIn(o.At, o.DateOnly, loc)
}

// daysAgo counts the calendar days in loc from o to now.
func (o Occurred) daysAgo(now time.Time, loc *time.Location) int {
	return occurredday.DaysSince(o.At, o.DateOnly, now, loc)
}

// daysUntil counts the calendar days from o to later, each read by its own
// precision.
func (o Occurred) daysUntil(later Occurred, loc *time.Location) int {
	return occurredday.DaysBetween(occurredday.Moment(o), occurredday.Moment(later), loc)
}
