// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// BaseValueSQL renders the deal under alias in the installation's base
// currency. A closed deal reads the amount frozen at close, so a corrected rate
// sheet never rewrites it. An open deal takes the latest rate on or before the
// installation's day at the as-of instant. A missing rate yields NULL, never a guessed rate of 1.
//
// asOfSQL is an instant, never a day cut in some other zone. asOfSQL and baseSQL are SQL expressions, a bind position or a report-engine
// token, so the caller keeps sole ownership of its argument slice.
//
// alias is interpolated into SQL, so it is a compile-time literal from the
// caller and never a name off a request.
func BaseValueSQL(asOfSQL, baseSQL, alias string) string {
	return BaseValueAtSQL(asOfSQL, "", baseSQL, alias)
}

// BaseValueAtSQL is BaseValueSQL reading the rate version that was current at
// ratesAtSQL, a timestamptz expression. A NULL or empty ratesAtSQL reads the
// current version. A new report then uses a corrected rate, and a drill-through
// from an older figure uses the rate that figure used.
func BaseValueAtSQL(asOfSQL, ratesAtSQL, baseSQL, alias string) string {
	return fmt.Sprintf(`CASE
		WHEN %[3]s.amount_minor IS NULL THEN NULL
		WHEN %[3]s.currency IS NULL OR %[3]s.currency = %[2]s THEN %[3]s.amount_minor
		WHEN %[3]s.amount_minor_base IS NOT NULL THEN %[3]s.amount_minor_base
		ELSE (SELECT CASE
		        -- Converted, and only when the result is a number the column can
		        -- hold. The comparison runs in numeric, where the product already
		        -- is, so it decides the question BEFORE a cast can raise it — and
		        -- an out-of-range cast does not return a wrong number, it aborts
		        -- the whole statement and takes the page with it.
		        --
		        -- NULL for a result too large, never a clamp to the biggest number
		        -- that fits, which is a lie with more digits. The Go engine answers
		        -- the same way (deals.ErrAmountOutOfRange, skipped by PriceAll), so
		        -- one implausible amount costs its own row and no more.
		        WHEN round(%[3]s.amount_minor * fr.rate
		                     * power(10::numeric, coalesce(
		                         (SELECT bd.digits FROM currency_minor_digits bd
		                           WHERE bd.currency = %[2]s), 2))
		                     / power(10::numeric, coalesce(
		                         (SELECT dd.digits FROM currency_minor_digits dd
		                           WHERE dd.currency = %[3]s.currency), 2)))
		             BETWEEN -9223372036854775808 AND 9223372036854775807
		        THEN round(%[3]s.amount_minor * fr.rate
		                     * power(10::numeric, coalesce(
		                         (SELECT bd.digits FROM currency_minor_digits bd
		                           WHERE bd.currency = %[2]s), 2))
		                     / power(10::numeric, coalesce(
		                         (SELECT dd.digits FROM currency_minor_digits dd
		                           WHERE dd.currency = %[3]s.currency), 2)))::bigint
		        ELSE NULL
		      END
		      FROM fx_rate fr
		      WHERE fr.from_currency = %[3]s.currency AND fr.to_currency = %[2]s
		        AND fr.rate_date <= %[4]s
		        AND %[5]s
		      ORDER BY fr.rate_date DESC LIMIT 1)
	END`, asOfSQL, baseSQL, alias, occurredday.TodaySQL(asOfSQL), RateVersionSQL("fr", ratesAtSQL))
}

// RateVersionSQL keeps the one version of each rate-sheet day that was current
// at ratesAtSQL. A correction supersedes the day's row and appends a new one,
// so every reader of fx_rate goes through this clause.
//
// Without ratesAtSQL it is the current row. alias is a compile-time literal;
// empty means the unaliased table.
func RateVersionSQL(alias, ratesAtSQL string) string {
	if alias != "" {
		alias += "."
	}
	current := alias + "superseded_at IS NULL"
	if ratesAtSQL == "" {
		return current
	}
	at := "(" + ratesAtSQL + ")::timestamptz"
	return "(CASE WHEN " + at + " IS NULL THEN " + current + " ELSE " +
		alias + "created_at <= " + at + " AND (" + alias + "superseded_at IS NULL OR " + alias + "superseded_at > " + at + ") END)"
}

// BaseValueSumSQL sums priced deals without making an absent price zero.
func BaseValueSumSQL(asOfSQL, baseSQL, alias, filterSQL string) string {
	return BaseValueSumAtSQL(asOfSQL, "", baseSQL, alias, filterSQL)
}

// BaseValueSumAtSQL is BaseValueSumSQL at a rate version, as BaseValueAtSQL is
// to BaseValueSQL.
func BaseValueSumAtSQL(asOfSQL, ratesAtSQL, baseSQL, alias, filterSQL string) string {
	return "sum(" + BaseValueAtSQL(asOfSQL, ratesAtSQL, baseSQL, alias) + ") FILTER (WHERE " + filterSQL + ")::bigint"
}

// UnpricedCountSQL counts amounts whose currency cannot be valued. An amountless
// deal has no money to price and contributes to neither sum nor coverage gap.
func UnpricedCountSQL(asOfSQL, baseSQL, alias, filterSQL string) string {
	return UnpricedCountAtSQL(asOfSQL, "", baseSQL, alias, filterSQL)
}

// UnpricedCountAtSQL is UnpricedCountSQL at a rate version.
func UnpricedCountAtSQL(asOfSQL, ratesAtSQL, baseSQL, alias, filterSQL string) string {
	return UnpricedValueCountSQL(BaseValueAtSQL(asOfSQL, ratesAtSQL, baseSQL, alias), alias+".amount_minor", filterSQL)
}

// UnpricedValueCountSQL keeps coverage under the same visibility as its amount.
func UnpricedValueCountSQL(valueSQL, amountSQL, filterSQL string) string {
	return "count(*) FILTER (WHERE " + filterSQL + " AND (" + amountSQL + ") IS NOT NULL AND (" + valueSQL + ") IS NULL)::integer"
}
