// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package moneytotal defines SQL for complete converted-money totals.
package moneytotal

// FinalizeBaseTotalSQL distinguishes no money from money that cannot be priced.
func FinalizeBaseTotalSQL(sumSQL, unpricedSQL string) string {
	return "CASE WHEN (" + sumSQL + ") IS NULL AND coalesce((" + unpricedSQL + "), 0) > 0 THEN NULL ELSE coalesce((" + sumSQL + "), 0) END"
}

// IncompleteBaseTotalSQL is a total that is NULL once any amount in it cannot
// be priced. It is for a figure that claims to be whole.
func IncompleteBaseTotalSQL(sumSQL, unpricedSQL string) string {
	return "CASE WHEN coalesce((" + unpricedSQL + "), 0) > 0 THEN NULL ELSE coalesce((" + sumSQL + "), 0) END"
}

// CompleteAggregateSQL withholds a partial group and preserves the empty
// aggregate's semantics. Only a sum has zero as its empty value.
func CompleteAggregateSQL(expression, unpricedSQL string, sum bool) string {
	if sum {
		return IncompleteBaseTotalSQL(expression, unpricedSQL)
	}
	return "CASE WHEN coalesce((" + unpricedSQL + "), 0) > 0 THEN NULL ELSE (" + expression + ") END"
}
