// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealcoverage says why a deal's money is missing from a converted
// total: no amount was entered, or the amount's currency has no rate. It
// renders the counts in the same statement as the sums they qualify.
package dealcoverage

import (
	"slices"
	"strings"
)

// Key is the row key a report puts its coverage under.
const Key = "coverage"

const (
	colNoValue    = "coverage_no_value"
	colNoRate     = "coverage_no_rate"
	colCurrencies = "coverage_currencies"
)

// NoValueSQL holds for a deal, under the table alias given, with no amount.
func NoValueSQL(alias string) string { return alias + ".amount_minor IS NULL" }

// NoRateSQL holds for a deal with an amount that baseValueSQL cannot convert.
// It excludes NoValueSQL, so a deal is counted under one cause only.
func NoRateSQL(alias, baseValueSQL string) string {
	return alias + ".amount_minor IS NOT NULL AND (" + baseValueSQL + ") IS NULL"
}

// Attach adds, after the plan's own terms and columns, the terms that count a
// group's deals left out of the partial sums named in covered. With none named
// the plan is returned as it came.
func Attach(covered []string, baseValueSQL, currencySQL string, selects, columns []string) (outSelects, outColumns []string) {
	if len(covered) == 0 {
		return selects, columns
	}
	noRate := NoRateSQL("t", baseValueSQL)
	terms := []string{
		"count(CASE WHEN " + NoValueSQL("t") + " THEN 1 END)",
		"count(CASE WHEN " + noRate + " THEN 1 END)",
		"string_agg(DISTINCT " + currencySQL + ", ',' ORDER BY " + currencySQL + ") FILTER (WHERE " + noRate + ")",
	}
	return append(slices.Clip(selects), terms...),
		append(slices.Clip(columns), colNoValue, colNoRate, colCurrencies)
}

// Fold moves each row's coverage columns into one `coverage` object naming the
// aggregates it qualifies.
func Fold(rows []map[string]any, aggregates []string) {
	if len(aggregates) == 0 {
		return
	}
	for _, row := range rows {
		noValue, noRate := count(row[colNoValue]), count(row[colNoRate])
		currencies := []string{}
		if listed, ok := row[colCurrencies].(string); ok && listed != "" {
			currencies = strings.Split(listed, ",")
		}
		delete(row, colNoValue)
		delete(row, colNoRate)
		delete(row, colCurrencies)
		row[Key] = map[string]any{
			"aggregates":         aggregates,
			"partial":            noValue+noRate > 0,
			"no_value_deals":     noValue,
			"no_rate_deals":      noRate,
			"missing_currencies": currencies,
		}
	}
}

//craft:ignore naked-any a driver row value of unknown integer width
func count(value any) int64 {
	switch n := value.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}
