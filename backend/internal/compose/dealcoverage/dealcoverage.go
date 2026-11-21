// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealcoverage names why a deal's money is missing from a converted
// total: no amount was entered, or the amount's currency has no rate.
package dealcoverage

// The measure and output names a report offers so a reader can see the
// shortfall beside a partial sum.
const (
	MeasureNoValue = "no_value_deal"
	MeasureNoRate  = "no_rate_deal"
	AliasNoValue   = "no_value_deals"
	AliasNoRate    = "no_rate_deals"
)

// NoValueSQL holds for a deal with no amount.
const NoValueSQL = "t.amount_minor IS NULL"

// NoRateSQL holds for a deal with an amount that baseValueSQL cannot convert.
// It excludes NoValueSQL, so a deal is counted under one cause only.
func NoRateSQL(baseValueSQL string) string {
	return "t.amount_minor IS NOT NULL AND (" + baseValueSQL + ") IS NULL"
}

// Measures are one-or-null per deal, so count(measure) is the number of deals
// with that cause.
func Measures(baseValueSQL string) map[string]string {
	return map[string]string{
		MeasureNoValue: "CASE WHEN " + NoValueSQL + " THEN 1 END",
		MeasureNoRate:  "CASE WHEN " + NoRateSQL(baseValueSQL) + " THEN 1 END",
	}
}
