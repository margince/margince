// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Money denominated PER ROW, and the one rule that makes summing it honest.
//
// Its own file because the rule and the declaration it reads have to be found
// together: a spec offering a native measure and forgetting to declare it gets
// no refusal at all, so the two are one idea rather than a check and a field.

import "slices"

// moneyDefaultBy is the DEFAULT grouping of a report whose default plan sums
// money: the report's own dimension, then currency (REPORT-VOCAB-1).
//
// Currency belongs in the default grouping and not merely in the vocabulary.
// amount_minor is a minor-unit integer in the deal's own currency, so a total
// spanning currencies is a number with no unit — the sum data-semantics §1 r4
// forbids outright and AC-DS-FX1 fails by construction. The default plan is
// the one an agent calls first and a screen renders unattended, so a currency
// split a caller has to opt into leaves that path adding dong to euros.
// Grouping makes every total mean something; converting to one base currency
// is the frozen-FX roll-up, a different and larger capability.
func moneyDefaultBy(dimension string) []string {
	return []string{dimension, fieldCurrency}
}

// refuseUngroupedNativeMoney holds "a native minor-unit sum is grouped by
// currency" as a RULE rather than as a default.
//
// The default plans already append currency, so an agent asking the default
// question gets a well-defined per-currency figure. But a caller naming its own
// group_by — which the MCP schema resource invites, listing group_by and
// aggregates as free choices — drops the split and the engine adds euros to
// dollars: two deals at 2,500,000 EUR and 1,000,000 USD answer 3,500,000 of
// nothing, where the honest converted total is 3,420,000 EUR.
//
// Refused rather than converted. Converting here would answer a question the
// caller did not ask, in a currency they did not name, and a plain integer that
// silently changed meaning is what this is about.
//
// The frontend already holds this rule for the plans the screen sends
// (analytics.test.tsx, "groups every native money plan by currency"). Two
// spellings of one invariant, and until now only one of them could fail.
func refuseUngroupedNativeMoney(
	spec reportSpec, groupBy []string, aggregates []reportAggregate,
) error {
	if len(spec.nativeMeasures) == 0 || slices.Contains(groupBy, fieldCurrency) {
		return nil
	}
	for _, agg := range aggregates {
		// count OF a measure is how many rows could compute it — a
		// dimensionless integer, not money. Counting euros and dollars
		// together answers "how many priced deals", which means the same thing
		// in every currency, so it needs no split. Every other function
		// COMBINES the values.
		if agg.Fn == aggFnCount {
			continue
		}
		if agg.Field != "" && spec.nativeMeasures[agg.Field] {
			return &NativeMoneyNeedsCurrencyError{Field: agg.Field}
		}
	}
	return nil
}

// nativeMoney names the measures a spec denominates per row.
//
// A constructor rather than a map literal per spec: four specs declare one, and
// the set is the whole point — a spec that offers a native measure and forgets
// to name it here is the defect, so the shortest possible spelling is what
// keeps that from happening by accident.
func nativeMoney(fields ...string) map[string]bool {
	out := make(map[string]bool, len(fields))
	for _, field := range fields {
		out[field] = true
	}
	return out
}
