// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"fmt"
	"time"
)

// PredictionKind says whether a frozen snapshot was a prediction anybody could
// have acted on.
type PredictionKind string

// The kinds. A snapshot taken at period close, or on a day after the period
// ended, recorded a result that was already known.
const (
	PredictionObserved     PredictionKind = "observed"
	PredictionAfterOutcome PredictionKind = "after_outcome"
)

// PredictionKindOf classifies a snapshot from columns frozen when it was taken.
// The local day is stored in the zone of that day, so a later change of the
// installation's zone cannot move a snapshot from one kind to the other.
func PredictionKindOf(trigger string, localDay, periodEnd time.Time) PredictionKind {
	if trigger == TriggerPeriodClose || !daysInOrder(localDay, periodEnd) {
		return PredictionAfterOutcome
	}
	return PredictionObserved
}

// HorizonDays is how many calendar days a snapshot looked ahead: zero on the
// period's last day, and -1 for any day after it.
func HorizonDays(localDay, periodEnd time.Time) int {
	return calendarDaysInclusive(localDay, periodEnd) - 1
}

// dayBefore answers whether one calendar day comes before another.
func dayBefore(earlier, later time.Time) bool {
	return daysInOrder(earlier, later) && !sameCalendarDay(earlier, later)
}

// PeriodOfDays rebuilds the period a snapshot stored as two local days, cut in
// zone. It refuses an end before the start.
func PeriodOfDays(startDay, endDay time.Time, zone *time.Location) (Period, error) {
	if zone == nil || endDay.Before(startDay) {
		return Period{}, fmt.Errorf("forecasting: %s to %s in zone %v is not a period",
			startDay.Format(time.DateOnly), endDay.Format(time.DateOnly), zone)
	}
	from := time.Date(startDay.Year(), startDay.Month(), startDay.Day(), 0, 0, 0, 0, zone)
	to := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), 0, 0, 0, 0, zone)
	return Period{Start: from, End: to.AddDate(0, 0, 1), StartDate: from, EndDate: to, Zone: zone}, nil
}
