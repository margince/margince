// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/shared/kernel/moneytotal"

// derivationColumns returns names and selects aligned by index. What a row may
// disclose is held by TestADrillThroughShowsWhatTheOrdinaryReadShowsAndNoMore.
func derivationColumns(spec reportSpec) (columns, selects []string) {
	columns, selects = []string{"id"}, []string{"t.id AS id"}
	add := func(name, expression string) {
		columns = append(columns, name)
		selects = append(selects, expression+" AS "+name)
	}
	for _, name := range sortedKeys(spec.dimensions) {
		add(name, spec.dimensions[name])
	}
	for _, name := range sortedKeys(spec.drillColumns) {
		add(name, spec.drillColumns[name])
	}
	for _, name := range sortedKeys(spec.measures) {
		expression := spec.measures[name]
		if coverage, ok := spec.sumCoverage[name]; ok {
			expression = moneytotal.CompleteAggregateSQL(expression, coverage, false)
		}
		add(name, expression)
	}
	return columns, selects
}

// derivationColumnExpr is the vocabulary expression behind a drill-through
// column, before any coverage wrapping: what a reference scope is keyed on.
func derivationColumnExpr(spec reportSpec, name string) (string, bool) {
	if expr, ok := spec.dimensions[name]; ok {
		return expr, true
	}
	if expr, ok := spec.drillColumns[name]; ok {
		return expr, true
	}
	expr, ok := spec.measures[name]
	return expr, ok
}
