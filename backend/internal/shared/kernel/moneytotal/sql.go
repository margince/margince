// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package moneytotal defines SQL for complete converted-money totals.
package moneytotal

import "strings"

// FinalizeBaseTotalSQL distinguishes no money from money that cannot be priced.
func FinalizeBaseTotalSQL(sumSQL, unpricedSQL string) string {
	return "CASE WHEN (" + sumSQL + ") IS NULL AND coalesce((" + unpricedSQL + "), 0) > 0 THEN NULL ELSE coalesce((" + sumSQL + "), 0) END"
}

// PartialSumSQL is the sum of the amounts a group can price. A group with
// deals but none it can price answers null. A group with no deals reads zero.
func PartialSumSQL(sumSQL string) string {
	return FinalizeBaseTotalSQL(sumSQL, "count(*)")
}

// NoValueSQL holds for a deal with no amount, as the reader may see it. A
// withheld amount counts as no value.
func NoValueSQL(amountSQL string) string { return "(" + amountSQL + ") IS NULL" }

// NoRateSQL holds for a deal with an amount that valueSQL cannot convert. It
// excludes NoValueSQL, so a deal is counted under one cause only.
func NoRateSQL(amountSQL, valueSQL string) string {
	return "(" + amountSQL + ") IS NOT NULL AND (" + valueSQL + ") IS NULL"
}

// CoverageTerms are three aggregate terms over a group of deals. They count
// the deals with no amount, count those with an amount valueSQL cannot convert,
// and list the currencies of those. onlySQL narrows which deals are counted, so
// a total over more deals than the reader may see can state only what they may.
func CoverageTerms(valueSQL, amountSQL, currencySQL, onlySQL string) []string {
	noRate := NoRateSQL(amountSQL, valueSQL)
	return []string{
		"count(*) FILTER (WHERE (" + onlySQL + ") AND " + NoValueSQL(amountSQL) + ")",
		"count(*) FILTER (WHERE (" + onlySQL + ") AND " + noRate + ")",
		"string_agg(DISTINCT " + currencySQL + ", ',' ORDER BY " + currencySQL + ") FILTER (WHERE (" + onlySQL + ") AND " + noRate + ")",
	}
}

// CoverageSQL is CoverageTerms as one SELECT list, over every deal in the group.
func CoverageSQL(valueSQL, amountSQL, currencySQL string) string {
	return strings.Join(CoverageTerms(valueSQL, amountSQL, currencySQL, "TRUE"), ", ")
}

// StrictBaseTotalSQL answers null for a group holding an amount that cannot be
// priced, rather than an aggregate over the rest of it.
//
// It is the form for an average, a minimum, a maximum or a percentile. They
// have no answer for a part of the group standing in for all of it.
func StrictBaseTotalSQL(aggregateSQL, unpricedSQL string) string {
	return "CASE WHEN coalesce((" + unpricedSQL + "), 0) > 0 THEN NULL ELSE (" + aggregateSQL + ") END"
}

// IncompleteBaseTotalSQL is StrictBaseTotalSQL for a sum, which reads an empty
// group as zero. It is for a figure that claims to be whole.
func IncompleteBaseTotalSQL(sumSQL, unpricedSQL string) string {
	return StrictBaseTotalSQL("coalesce(("+sumSQL+"), 0)", unpricedSQL)
}

// CompleteAggregateSQL withholds a partial group and preserves the empty
// aggregate's semantics. Only a sum has zero as its empty value.
func CompleteAggregateSQL(expression, unpricedSQL string, sum bool) string {
	if sum {
		return IncompleteBaseTotalSQL(expression, unpricedSQL)
	}
	return StrictBaseTotalSQL(expression, unpricedSQL)
}
