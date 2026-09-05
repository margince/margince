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

// The window lengths a forecast is read over.
const (
	PeriodQuarter PeriodKind = "quarter"
	PeriodMonth   PeriodKind = "month"
	// PeriodWeek is the working week, Monday to Sunday. Unlike the other two it
	// is not a division of the financial year — a fiscal year opening in April
	// moves every quarter and month boundary and moves no Monday — so it is
	// resolved by ResolveWeek rather than by ResolvePeriod.
	PeriodWeek PeriodKind = "week"
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
	// A week is not a division of the financial year, so the month arithmetic
	// below cannot answer it. Refused rather than defaulted: monthsIn returns 3
	// for anything it does not recognise, so an unrefused week would resolve to
	// a QUARTER and every reading would silently be the wrong window.
	if kind == PeriodWeek {
		return Period{}, fmt.Errorf(
			"forecasting: a week is resolved by ResolveWeek from the Monday weekly.WeekStartOf returns, " +
				"not from the financial year")
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

// ResolveWeek builds the seven-day window opening on the Monday handed in.
//
// It takes the Monday rather than finding one, and that is the whole point.
// Which day a week opens on is already decided by weekly.WeekStartOf, which
// derives it through the installation's own reporting zone; a second spelling
// here would be a second answer to "what week is it", and the two would file a
// Sunday-night job's work under different weeks.
//
// So this refuses anything that is not a Monday at local midnight rather than
// rounding to the nearest one. A caller who has the wrong instant gets an error
// they can read, instead of a window silently shifted by a day — which would
// put a deal in one week's readings and out of the next.
//
// SEVEN LOCAL DAYS, not 168 hours. AddDate crosses a daylight-saving boundary
// by keeping the wall clock, so the spring week is 167 hours and the autumn one
// 169, and both are still the seven days a person worked. Adding a duration
// would leave the autumn week ending an hour before Sunday closed.
func ResolveWeek(monday time.Time, zone *time.Location) (Period, error) {
	if zone == nil {
		return Period{}, fmt.Errorf("forecasting: a period needs the installation zone, which decides which day an instant falls on")
	}
	local := monday.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	if !start.Equal(local) {
		return Period{}, fmt.Errorf(
			"forecasting: a week opens at local midnight, and %s is not one — "+
				"pass the Monday weekly.WeekStartOf returned rather than an instant inside the day",
			local.Format(time.RFC3339))
	}
	if start.Weekday() != time.Monday {
		return Period{}, fmt.Errorf(
			"forecasting: a week opens on a Monday, and %s is a %s — "+
				"which day a week starts on is weekly.WeekStartOf's answer, not this function's",
			start.Format("2006-01-02"), start.Weekday())
	}
	end := start.AddDate(0, 0, 7)
	return Period{
		Start:     start,
		End:       end,
		StartDate: start,
		EndDate:   end.AddDate(0, 0, -1),
		Zone:      zone,
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
//
// Compared as CALENDAR DAYS rather than as instants, because the two sides
// carry their midnight in different zones. `deal.expected_close_date` is a
// Postgres `date` and pgx scans it as UTC midnight; StartDate and EndDate are
// midnight in the installation's zone. Comparing those instants asks whether
// one moment precedes another, which is not the question — and it answers
// wrongly at exactly the ends this window is inclusive at.
//
// Nine hours east, a deal expected on the window's last day scans as 09:00 on
// that day and reads as AFTER an EndDate of 00:00, so it falls out of the
// period it belongs to. West of UTC the same arithmetic drops the first day
// instead. Both vanish from the reading with nothing to say where they went,
// which is the failure Period's own doc comment describes for the other column.
func (p Period) ContainsDay(day time.Time) bool {
	return daysInOrder(p.StartDate, day) && daysInOrder(day, p.EndDate)
}

// daysInOrder answers whether one calendar day is the same as or before
// another, ignoring what time of day and what zone each carries.
func daysInOrder(earlier, later time.Time) bool {
	ey, em, ed := earlier.Date()
	ly, lm, ld := later.Date()
	if ey != ly {
		return ey < ly
	}
	if em != lm {
		return em < lm
	}
	return ed <= ld
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
