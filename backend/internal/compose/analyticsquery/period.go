// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/reportperiod"
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
type PeriodGrain = reportperiod.Grain

// The three grains a period field is cut in.
const (
	GrainYear    = reportperiod.GrainYear
	GrainQuarter = reportperiod.GrainQuarter
	GrainMonth   = reportperiod.GrainMonth
)

// PeriodField marks a field whose value is a period bucket's label. Anchor is
// the SQL of the instant that places a row in its bucket.
type PeriodField struct {
	Anchor string
	Grain  PeriodGrain
}

// PeriodSpan answers the half-open interval a bucket label names under one
// basis. It reads the label with reportperiod.ParseLabel, as the report
// engine's filter check does, so the two accept the same spellings.
func PeriodSpan(grain PeriodGrain, label string, basis CalendarBasis) (from, to time.Time, err error) {
	zone, zoneErr := time.LoadLocation(basis.Timezone)
	if zoneErr != nil {
		return from, to, unresolvedPeriod(label)
	}
	from, to, ok := reportperiod.Span(grain, label, zone, basis.FiscalStartMonth)
	if !ok {
		return from, to, unresolvedPeriod(label)
	}
	return from, to, nil
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
