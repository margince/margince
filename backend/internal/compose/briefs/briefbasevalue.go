// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// One deal's money in the installation's base currency, as SQL.
//
// Its own file because it is a SECOND SPELLING of compose.BaseValueSQL, held
// character-identical to it by a gate, and a reader who finds it inside the
// ranker reads it as the ranker's own arithmetic rather than as one half of a
// pair that must move together.

import "fmt"

// briefBaseValueSQL renders the §6 base-currency value of d (joined to
// its workspace w): native amount when already in base currency, the
// frozen amount_minor_base (written by the freeze writer at close, across
// both currencies' minor-unit scales) for closed deals, the
// latest daily rate on or before the as-of date for open ones. A missing
// rate yields NULL — the revenue factor floors rather than guessing (a
// wrong number is worse than a missing one). asOfPos is the bind position
// of the as-of date.
// THE SECOND SPELLING, AND WHY. compose.BaseValueSQL is the same expression.
// This package cannot call it — compose imports briefs, so the reverse is a
// cycle — so the two are held character-identical by
// TestOneSpellingOfADealsBaseValue rather than left to drift.
func briefBaseValueSQL(asOfSQL, baseSQL, alias string) string {
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
