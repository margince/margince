// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// CalendarBasis is the calendar a period bucket was named under: the reporting
// zone and the month the fiscal year began. A filter that carries one keeps
// the dates its bucket covered then. One that carries none follows the
// current calendar.
type CalendarBasis struct {
	Timezone         string
	FiscalStartMonth int
}

// PeriodGrain is how long one bucket of a period field is.
type PeriodGrain string

// The three grains a period field is cut in.
const (
	GrainYear    PeriodGrain = "year"
	GrainQuarter PeriodGrain = "quarter"
	GrainMonth   PeriodGrain = "month"
)

// PeriodField marks a field whose value is a period bucket's label. Anchor is
// the SQL of the instant that places a row in its bucket.
type PeriodField struct {
	Anchor string
	Grain  PeriodGrain
}

var (
	calendarYearLabel    = regexp.MustCompile(`^(\d{4})$`)
	fiscalYearLabel      = regexp.MustCompile(`^FY(\d{4})/\d{2}$`)
	calendarQuarterLabel = regexp.MustCompile(`^(\d{4})-Q([1-4])$`)
	fiscalQuarterLabel   = regexp.MustCompile(`^FY(\d{4})/\d{2}-Q([1-4])$`)
	monthLabel           = regexp.MustCompile(`^(\d{4})-(0[1-9]|1[0-2])$`)
)

// PeriodSpan answers the half-open interval a bucket label names under one
// basis. The report engine spells a January year `2026` and any other
// `FY2026/27`. A label in the other spelling names no period under that basis,
// so it is refused.
func PeriodSpan(grain PeriodGrain, label string, basis CalendarBasis) (from, to time.Time, err error) {
	zone, zoneErr := time.LoadLocation(basis.Timezone)
	if zoneErr != nil || basis.FiscalStartMonth < 1 || basis.FiscalStartMonth > 12 {
		return from, to, unresolvedPeriod(label)
	}
	year, month, months, ok := periodStart(grain, label, basis.FiscalStartMonth)
	if !ok {
		return from, to, unresolvedPeriod(label)
	}
	from = time.Date(year, time.Month(month), 1, 0, 0, 0, 0, zone)
	return from, from.AddDate(0, months, 0), nil
}

// periodStart reads a label as the year and month its bucket begins in, and
// how many months it lasts. A month past December rolls into the next year
// when the date is built.
func periodStart(grain PeriodGrain, label string, fiscalStart int) (year, month, months int, ok bool) {
	fiscal := fiscalStart != 1
	switch grain {
	case GrainYear:
		pattern := calendarYearLabel
		if fiscal {
			pattern = fiscalYearLabel
		}
		parts := pattern.FindStringSubmatch(label)
		if parts == nil {
			return 0, 0, 0, false
		}
		return atoi(parts[1]), fiscalStart, 12, true
	case GrainQuarter:
		pattern := calendarQuarterLabel
		if fiscal {
			pattern = fiscalQuarterLabel
		}
		parts := pattern.FindStringSubmatch(label)
		if parts == nil {
			return 0, 0, 0, false
		}
		return atoi(parts[1]), fiscalStart + 3*(atoi(parts[2])-1), 3, true
	case GrainMonth:
		parts := monthLabel.FindStringSubmatch(label)
		if parts == nil {
			return 0, 0, 0, false
		}
		return atoi(parts[1]), atoi(parts[2]), 1, true
	}
	return 0, 0, 0, false
}

// atoi reads digits the label pattern already matched.
func atoi(digits string) int {
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return n
}

func unresolvedPeriod(label string) error {
	return &RefusalError{
		Kind:    RefusalInvalid,
		Message: fmt.Sprintf("the saved period %s names no period in the calendar it was saved under", httperr.QuoteCaller(label)),
		Suggest: "pick the period again",
	}
}

// periodClause renders a filter on a period field that keeps its basis as a
// range over the anchor. The labels sort in time order, so each comparison
// keeps its meaning. Before a bucket is before its start. After it is from its
// end on.
//
//craft:ignore naked-any filter values are the decoded JSON plan, schemaless by design
func periodClause(period PeriodField, f Filter, arg func(any) string) (string, error) {
	label, isText := f.Value.(string)
	if !isText || f.Basis == nil {
		return "", unresolvedPeriod(fmt.Sprint(f.Value))
	}
	from, to, err := PeriodSpan(period.Grain, label, *f.Basis)
	if err != nil {
		return "", err
	}
	anchor := period.Anchor
	switch f.Op {
	case OpEq:
		return "(" + anchor + " >= " + arg(from) + " AND " + anchor + " < " + arg(to) + ")", nil
	case OpNe:
		return "(" + anchor + " < " + arg(from) + " OR " + anchor + " >= " + arg(to) + ")", nil
	case OpLt:
		return anchor + " < " + arg(from), nil
	case OpLte:
		return anchor + " < " + arg(to), nil
	case OpGt:
		return anchor + " >= " + arg(to), nil
	case OpGte:
		return anchor + " >= " + arg(from), nil
	}
	return "", unresolvedPeriod(label)
}

// StampBasis gives every filter on a period field, with a value to compare,
// the basis it is asked under now. A filter that already keeps one is left as
// it is.
func StampBasis(filters []Filter, isPeriod func(field string) bool, basis CalendarBasis) []Filter {
	out := make([]Filter, len(filters))
	for i, f := range filters {
		if isPeriod(f.Field) && f.Basis == nil && !valuelessOps[f.Op] {
			kept := basis
			f.Basis = &kept
		}
		out[i] = f
	}
	return out
}
