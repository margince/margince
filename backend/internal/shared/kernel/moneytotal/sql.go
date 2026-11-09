// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package moneytotal defines SQL for complete converted-money totals.
package moneytotal

// FinalizeBaseTotalSQL distinguishes no money from money that cannot be priced.
func FinalizeBaseTotalSQL(sumSQL, unpricedSQL string) string {
	return "CASE WHEN (" + sumSQL + ") IS NULL AND coalesce((" + unpricedSQL + "), 0) > 0 THEN NULL ELSE coalesce((" + sumSQL + "), 0) END"
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
