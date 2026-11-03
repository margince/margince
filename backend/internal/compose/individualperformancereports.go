// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The individual-performance switch on the report engine's own vocabulary:
// which columns name a seat, and which questions over them are about a colleague.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatColumns are the report expressions whose value is a seat. The owner
// is the column a population narrows on. The other two credit a meeting to
// the colleague who held it.
//
// Held by: TestEveryReportColumnNamingASeatAnswersToTheSwitch, which derives
// every seat column from the schema and fails on a spec column reading one
// that is missing here.
var seatColumns = map[string]bool{
	colOwnerID:             true,
	colMeetingCreditedSeat: true,
	colCreditedSeat:        true,
}

func namesSeat(spec reportSpec, field string) bool {
	return seatColumns[spec.dimensions[field]] || seatColumns[spec.filters[field]]
}

// seatFilter is one filter of any door, reduced to what the switch judges.
// exact is false for a comparison that ranges over seats.
type seatFilter struct {
	field string
	exact bool
	//craft:ignore naked-any the value is the decoded JSON plan's, schemaless by design
	value any
}

//craft:ignore naked-any filter values are the decoded JSON plan, schemaless by design
func equalityFilters(filters map[string]any) []seatFilter {
	out := make([]seatFilter, 0, len(filters))
	for field, value := range filters {
		out = append(out, seatFilter{field: field, exact: true, value: value})
	}
	return out
}

func typedFilters(filters []analyticsquery.Filter) []seatFilter {
	out := make([]seatFilter, 0, len(filters))
	for _, f := range filters {
		out = append(out, seatFilter{field: f.Field, exact: exactOwnerComparisons[f.Op], value: f.Value})
	}
	return out
}

// requireIndividualQuestion refuses, while the switch is off, a question that
// names a colleague or splits its answer by seat. A grouping stays allowed
// when it is pinned to the reader or the population is the reader alone. It
// also stays when it lists each record by its own id. A listing names a
// record's owner as the record's page does, and measures nobody.
func requireIndividualQuestion(
	ctx context.Context, tx pgx.Tx, spec reportSpec, requested RequestedScope, groupBy []string, filters []seatFilter,
) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return err
	}
	pinned := map[string]bool{}
	for _, f := range filters {
		if !namesSeat(spec, f.field) {
			continue
		}
		if !f.exact {
			return auth.IndividualPerformanceOffError{}
		}
		seat, named := filterSeat(f.value)
		if !named {
			continue
		}
		if !auth.MayMeasureSeat(ctx, false, seat) {
			return auth.IndividualPerformanceOffError{}
		}
		expr := spec.filters[f.field]
		if expr == "" {
			expr = spec.dimensions[f.field]
		}
		pinned[expr] = true
	}
	return requireOwnGrouping(ctx, tx, spec, requested, groupBy, pinned)
}

// requireNamedQuestion runs, for both doors onto a report, every gate a plan's
// filter values and grouping answer to before anything binds.
//
//craft:ignore naked-any filter values are the decoded JSON plan, schemaless by design
func requireNamedQuestion(
	ctx context.Context, tx pgx.Tx, spec reportSpec, requested RequestedScope, groupBy []string, filters map[string]any,
) error {
	if err := requireNamedRecords(ctx, tx, spec, requested, filters); err != nil {
		return err
	}
	return requireIndividualQuestion(ctx, tx, spec, requested, groupBy, equalityFilters(filters))
}

// filterSeat reads the seat a filter value names. A value that is no id
// names nobody, and the query refuses it as the caller's mistyped value.
//
//craft:ignore naked-any the value is the decoded JSON plan's, schemaless by design
func filterSeat(value any) (ids.UUID, bool) {
	text, ok := value.(string)
	if !ok {
		return ids.UUID{}, false
	}
	seat, err := ids.Parse(text)
	return seat, err == nil
}

func requireOwnGrouping(
	ctx context.Context, tx pgx.Tx, spec reportSpec, requested RequestedScope, groupBy []string, pinned map[string]bool,
) error {
	bySeat, byOwnerOnly, byRow := false, true, false
	for _, dim := range groupBy {
		expr := spec.dimensions[dim]
		switch {
		case expr == colRowID:
			byRow = true
		case seatColumns[expr] && !pinned[expr]:
			bySeat = true
			byOwnerOnly = byOwnerOnly && expr == colOwnerID
		}
	}
	if !bySeat || byRow {
		return nil
	}
	if byOwnerOnly {
		alone, err := populationIsReader(ctx, tx, requested)
		if err != nil || alone {
			return err
		}
	}
	return auth.IndividualPerformanceOffError{}
}

// populationIsReader answers whether an owner breakdown can show only the
// reader. The population resolves to them, which is also where the owner gate
// narrows an own-lens seat's unscoped breakdown.
func populationIsReader(ctx context.Context, tx pgx.Tx, requested RequestedScope) (bool, error) {
	p, ok := principal.Actor(ctx)
	if !ok {
		return false, errors.New("compose: no actor bound to context")
	}
	resolved, err := resolveAnalyticsScope(ctx, tx, p, requested)
	if err != nil {
		return false, err
	}
	return resolved.Kind == ScopeKindOwner && resolved.ID != nil && *resolved.ID == p.UserID, nil
}
