// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"fmt"
	"time"
)

// Period is one forecasting window, resolved in the installation's own zone and
// against its own financial year.
//
// It carries the SAME window twice, in two types, because the two columns it is
// compared against are two types. `deal.expected_close_date` is a zoneless
// `date`: a person wrote down a day, and it means that day everywhere.
// `deal.closed_at` is a `timestamptz`: an instant, which falls on different
// days depending on where you stand.
//
// Comparing them with one bound is the bug this type exists to prevent.
// A `date` compared against a `timestamptz` bound is cast at the SESSION
// timezone, which on a worker connection is not the installation's. A deal
// closing just after local midnight on the first day of the next quarter would
// then leave the evidence reading (its expected date sits in this period)
// without entering the won reading (its close instant reads as next period),
// and it would vanish from both — from the headline, and from the one-sentence
// answer, with no bucket to explain where it went.
//
// The rule every reading applies: an expected close date is compared as a
// LOCAL CALENDAR DATE against StartDate/EndDate, and a close instant is
// converted to a local date first, then compared the same way.
//
// Held by: TestAnInstantJustPastLocalMidnightBelongsToTheLocalDay
// (periods_test.go), which fails if either comparison stops going through the
// installation zone.
type Period struct {
	// The half-open instant bounds, [Start, End). For anything that is genuinely
	// an instant and is compared as one.
	Start time.Time
	End   time.Time
	// The closed local-day bounds, [StartDate, EndDate]. Inclusive at both
	// ends, because a calendar quarter's last day is IN the quarter and a
	// half-open date range would need the caller to know the next day's date to
	// express that.
	StartDate time.Time
	EndDate   time.Time
	// The zone the days were cut in, carried so a caller converting a
	// timestamptz uses the same one this period was built with rather than
	// reaching for a second source of the answer.
	Zone *time.Location
}

// PeriodKind is the length of a forecasting window.
type PeriodKind string

// The two window lengths a forecast is read over.
const (
	PeriodQuarter PeriodKind = "quarter"
	PeriodMonth   PeriodKind = "month"
)

// ResolvePeriod answers which window `at` falls in, for an installation whose
// financial year opens in fiscalStartMonth and whose days are cut in zone.
//
// fiscalStartMonth is 1..12 and 1 means the calendar year, which is what most
// installations are. A year opening in April shifts every quarter boundary by
// three months: its Q1 is April to June, and a deal closing in May belongs to
// the first quarter of a financial year that started before it.
func ResolvePeriod(kind PeriodKind, at time.Time, fiscalStartMonth int, zone *time.Location) (Period, error) {
	if zone == nil {
		return Period{}, fmt.Errorf("forecasting: a period needs the installation zone, which decides which day an instant falls on")
	}
	if fiscalStartMonth < 1 || fiscalStartMonth > 12 {
		return Period{}, fmt.Errorf("forecasting: fiscal year start month %d is outside 1..12", fiscalStartMonth)
	}
	local := at.In(zone)
	startYear, startMonth := periodStart(kind, local, fiscalStartMonth)
	start := time.Date(startYear, startMonth, 1, 0, 0, 0, 0, zone)
	end := start.AddDate(0, monthsIn(kind), 0)
	return Period{
		Start:     start,
		End:       end,
		StartDate: start,
		// The last day INSIDE the window: one day before the exclusive instant
		// bound. Derived rather than written, so a leap day or a 31-day month
		// cannot be got wrong by arithmetic on a day count.
		EndDate: end.AddDate(0, 0, -1),
		Zone:    zone,
	}, nil
}

// consistent answers whether a Period's two spellings describe one window.
//
// ResolvePeriod always builds them together, so this only ever fails for a
// Period a caller assembled by hand — which is exactly the case worth refusing,
// because the two halves are read by different code paths and a disagreement
// puts a deal in one reading and not the other.
func (p Period) consistent() bool {
	if p.Zone == nil {
		return false
	}
	return p.StartDate.Equal(p.Start) && p.EndDate.Equal(p.End.AddDate(0, 0, -1))
}

// LocalDay reduces an instant to the calendar day it fell on IN THIS PERIOD'S
// ZONE — the conversion every close-instant comparison goes through.
//
// Returned as a midnight time in the period's zone rather than a string,
// because the comparison below is against StartDate and EndDate, which are the
// same shape. A day rendered as text would compare lexically, which is right by
// accident for ISO dates and wrong the moment anything else is passed.
func (p Period) LocalDay(at time.Time) time.Time {
	local := at.In(p.Zone)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, p.Zone)
}

// ContainsDay answers whether a local calendar day falls in this window.
// Inclusive at both ends: the last day of a quarter is in the quarter.
func (p Period) ContainsDay(day time.Time) bool {
	return !day.Before(p.StartDate) && !day.After(p.EndDate)
}

// ContainsInstant answers whether an instant falls in this window, by the day it
// landed on locally. NOT a comparison against Start and End: those are the same
// window and would answer the same question correctly, but going through the
// local day keeps ONE rule for both column types, and a second correct path is
// how the two come to disagree the day somebody edits one of them.
func (p Period) ContainsInstant(at time.Time) bool {
	return p.ContainsDay(p.LocalDay(at))
}

func monthsIn(kind PeriodKind) int {
	if kind == PeriodMonth {
		return 1
	}
	return 3
}

// periodStart answers the year and month a window opens in.
func periodStart(kind PeriodKind, local time.Time, fiscalStartMonth int) (int, time.Month) {
	if kind == PeriodMonth {
		return local.Year(), local.Month()
	}
	// Months elapsed since the financial year opened, which is what decides
	// which quarter of THAT year we are in. The +12 keeps the modulo positive
	// for a date before the current calendar year's opening month.
	elapsed := (int(local.Month()) - fiscalStartMonth + 12) % 12
	// Walk back from the first of this month by the months into the quarter,
	// rather than doing month arithmetic that can land on a day the target
	// month does not have (the 31st of a 30-day month rolls forward).
	firstOfMonth := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	opening := firstOfMonth.AddDate(0, -(elapsed % 3), 0)
	return opening.Year(), opening.Month()
}
