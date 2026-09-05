// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One deal's money in the installation's base currency, as SQL.
//
// Three cases and they are not interchangeable. A deal already in the base
// currency needs no rate. A CLOSED deal carries the rate it closed at, frozen
// on the row, and re-converting it at today's rate would rewrite history every
// time a rate sheet was corrected. An open deal takes the latest daily rate on
// or before the as-of date.
//
// A missing rate yields NULL and never a rate of 1. A guessed number is worse
// than an absent one here: it looks like pipeline, sums into a headline, and
// nothing downstream can tell it from money somebody actually expects.

// THE SECOND SPELLING, AND WHY. compose/briefs holds the same expression as
// briefBaseValueSQL. It cannot call this one: compose imports briefs, so briefs
// importing compose is a cycle. Moving the pair down a tier is a real option and
// a separate change. Until then the two are held CHARACTER-IDENTICAL by
// TestOneSpellingOfADealsBaseValue, which fails in both directions — a rate rule
// changed in one place and not the other is exactly the drift a forecast and a
// morning brief must never show a reader.

import "fmt"

// BaseValueSQL renders the expression for the deal under `alias`.
//
// asOfSQL and baseSQL are SQL EXPRESSIONS carrying the as-of date and the base
// currency — a bind position a caller registered itself, or a token the report
// engine substitutes one for when it assembles the statement. Expressions
// rather than positions because the caller owns its argument slice: a helper
// that appended to it would have to be called in a particular order to be
// correct, and the report engine does not know its positions until the whole
// statement is built.
//
// alias is interpolated into SQL, so it is a compile-time literal from the
// calling spec and never a name off a request. It is a parameter because the
// forecast reads the deal as `d` and the report engine as `t`, and a second
// copy of this expression differing only in a letter is exactly the drift the
// gate below exists to prevent.
func BaseValueSQL(asOfSQL, baseSQL, alias string) string {
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
		        AND fr.rate_date <= %[1]s::date
		      ORDER BY fr.rate_date DESC LIMIT 1)
	END`, asOfSQL, baseSQL, alias)
}
