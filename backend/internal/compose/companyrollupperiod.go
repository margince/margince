// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/reportperiod"
)

// CompanyRollupPeriod is the period a caller names for the rollup's won sales:
// one period field (period_year, period_quarter or period_month) and its
// bucket label. The zero value names none, which reads the fiscal quarter
// under way.
type CompanyRollupPeriod struct {
	Field string
	Label string
}

// RollupWonPeriod is the window closed-won was summed over, with the labels a
// reader steps to. Next is nil while the period after this one has not begun.
type RollupWonPeriod struct {
	Grain      reportperiod.Grain
	Label      string
	Start, End time.Time
	Chosen     bool
	Previous   string
	Next       *string
}

// rollupPeriodRefusal refuses one period parameter. It names the parameter on
// every surface, and unwraps to ErrInvalidArgument for callers that branch on
// the sentinel.
type rollupPeriodRefusal struct{ field, message string }

func (e *rollupPeriodRefusal) Error() string { return e.message }
func (e *rollupPeriodRefusal) Unwrap() error { return apperrors.ErrInvalidArgument }
func (e *rollupPeriodRefusal) FieldFault() (field, code, message string) {
	return e.field, "invalid", e.message
}

// NewCompanyRollupPeriod reads the three optional period parameters. Naming
// more than one is refused, since the rollup sums one window.
func NewCompanyRollupPeriod(year, quarter, month *string) (CompanyRollupPeriod, error) {
	var named []CompanyRollupPeriod
	for _, p := range []struct {
		field string
		label *string
	}{{fieldPeriodYear, year}, {fieldPeriodQuarter, quarter}, {fieldPeriodMonth, month}} {
		if p.label != nil {
			named = append(named, CompanyRollupPeriod{Field: p.field, Label: *p.label})
		}
	}
	switch len(named) {
	case 0:
		return CompanyRollupPeriod{}, nil
	case 1:
		return named[0], nil
	}
	return CompanyRollupPeriod{}, &rollupPeriodRefusal{field: named[1].Field,
		message: "name one of period_year, period_quarter or period_month, not several"}
}

// periodAt is one instant whose bucket label is wanted at one period field.
type periodAt struct {
	field string
	at    time.Time
}

// rollupPeriodLabels renders each instant's bucket label in one statement,
// with the expression the period reports group by. Rendering it anywhere else
// would be a second answer to what `FY2025/26-Q4` means.
func rollupPeriodLabels(ctx context.Context, tx pgx.Tx, zone *time.Location, fiscalStart int, wanted ...periodAt) ([]string, error) {
	args := make([]any, 0, len(wanted))
	columns := make([]string, 0, len(wanted))
	for _, w := range wanted {
		args = append(args, w.at)
		columns = append(columns, periodDimensions(fmt.Sprintf("$%d::timestamptz", len(args)))[w.field])
	}
	frame := reportFrame{Timezone: zone.String(), FiscalYearStartMonth: fiscalStart}
	sql, args, err := bindReportTokens(ctx, tx, frame, "SELECT "+strings.Join(columns, ", "), args)
	if err != nil {
		return nil, err
	}
	labels := make([]string, len(wanted))
	targets := make([]any, len(labels))
	for i := range labels {
		targets[i] = &labels[i]
	}
	if err := tx.QueryRow(ctx, sql, args...).Scan(targets...); err != nil {
		return nil, fmt.Errorf("compose: naming the rollup's periods: %w", err)
	}
	return labels, nil
}

// rollupCurrentPeriods names the year, quarter and month under way at asOf.
func rollupCurrentPeriods(ctx context.Context, tx pgx.Tx, asOf time.Time, zone *time.Location, fiscalStart int) (map[string]string, error) {
	fields := []string{fieldPeriodYear, fieldPeriodQuarter, fieldPeriodMonth}
	wanted := make([]periodAt, len(fields))
	for i, field := range fields {
		wanted[i] = periodAt{field: field, at: asOf}
	}
	labels, err := rollupPeriodLabels(ctx, tx, zone, fiscalStart, wanted...)
	if err != nil {
		return nil, err
	}
	current := make(map[string]string, len(fields))
	for i, field := range fields {
		current[field] = labels[i]
	}
	return current, nil
}

// resolveRollupPeriod turns the named period, or the quarter under way, into
// its half-open window. The label goes through reportperiod.Span, the reader a
// saved period filter uses, so the rollup and a report accept the same
// spellings. A period that has not begun is refused: it holds no won deal, and
// a zero for it would read as a measured result.
func resolveRollupPeriod(ctx context.Context, tx pgx.Tx, period CompanyRollupPeriod, current map[string]string,
	asOf time.Time, zone *time.Location, fiscalStart int,
) (RollupWonPeriod, error) {
	won := RollupWonPeriod{Label: period.Label, Chosen: period.Field != ""}
	field := period.Field
	if !won.Chosen {
		field, won.Label = fieldPeriodQuarter, current[fieldPeriodQuarter]
	}
	won.Grain = periodGrains[field]
	start, end, ok := reportperiod.Span(won.Grain, won.Label, zone, fiscalStart)
	if !ok {
		return won, &rollupPeriodRefusal{field: field, message: fmt.Sprintf(
			"%s names no %s in this installation's calendar; spell it as the period reports do, such as %s",
			httperr.QuoteCaller(won.Label), won.Grain, current[field])}
	}
	if start.After(asOf) {
		return won, &rollupPeriodRefusal{field: field, message: fmt.Sprintf(
			"%s has not begun yet; name a period up to %s", httperr.QuoteCaller(won.Label), current[field])}
	}
	won.Start, won.End = start, end
	labels, err := rollupPeriodLabels(ctx, tx, zone, fiscalStart,
		periodAt{field, start.Add(-time.Microsecond)}, periodAt{field, end})
	if err != nil {
		return won, err
	}
	won.Previous = labels[0]
	if !end.After(asOf) {
		won.Next = &labels[1]
	}
	return won, nil
}
