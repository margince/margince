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

// BaseValueSQL renders the expression for the deal aliased `d`.
//
// asOfPos and basePos are the bind positions of the as-of date and the base
// currency in the caller's own argument list. Positions rather than values
// because the caller owns the args slice, and a helper that appended to it
// would have to be called in a particular order to be correct.
func BaseValueSQL(asOfPos, basePos int) string {
	return fmt.Sprintf(`CASE
		WHEN d.amount_minor IS NULL THEN NULL
		WHEN d.currency IS NULL OR d.currency = $%[2]d THEN d.amount_minor
		WHEN d.fx_rate_to_base IS NOT NULL THEN d.amount_minor_base
		ELSE (SELECT round(d.amount_minor * fr.rate)::bigint FROM fx_rate fr
		      WHERE fr.from_currency = d.currency AND fr.to_currency = $%[2]d
		        AND fr.rate_date <= $%[1]d::date
		      ORDER BY fr.rate_date DESC LIMIT 1)
	END`, asOfPos, basePos)
}
