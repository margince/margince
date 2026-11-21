// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Money denominated PER ROW, and the one rule that makes summing it honest.
//
// The rule reads each measure's SQL, so no spec declares which measures it covers.

import (
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
)

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

// refuseUngroupedNativeMoney holds the currency rule for a caller naming its own
// group_by: two deals at 2,500,000 EUR and 1,000,000 USD must not answer
// 3,500,000 of nothing. Refused rather than converted, because converting
// answers a question in a currency the caller did not name. The rule is
// analyticsquery.PerRowCurrency, shared with the typed query.
func refuseUngroupedNativeMoney(
	spec reportSpec, groupBy []string, filters map[string]any, aggregates []reportAggregate,
) error {
	for _, agg := range aggregates {
		money := spec.perRowCurrency(agg.Field)
		if money != nil && money.Mixes(analyticsquery.AggFn(agg.Fn), groupBy, filters[money.By]) {
			return &NativeMoneyNeedsCurrencyError{Field: agg.Field, refusal: money.Refusal(analyticsquery.AggFn(agg.Fn), agg.Field)}
		}
	}
	return nil
}

// convertedMeasure names, for each per-row money measure, the measure holding
// the same amount in the installation's base currency.
var convertedMeasure = map[string]string{
	fieldAmountMinor:         fieldAmountBaseMinor,
	fieldWeightedAmountMinor: fieldWeightedBaseMinor,
}

// perRowCurrency says how a measure of this spec is denominated, nil unless its
// SQL is per-row money. It names the converted measure only when this spec
// offers it, so a refusal never points at a measure the caller cannot ask for.
func (s reportSpec) perRowCurrency(measure string) *analyticsquery.PerRowCurrency {
	if !perRowMoneySQL(s.measures[measure]) {
		return nil
	}
	money := &analyticsquery.PerRowCurrency{By: fieldCurrency}
	if base, ok := convertedMeasure[measure]; ok {
		if _, offered := s.measures[base]; offered {
			money.Base = base
		}
	}
	return money
}

// perRowMoneySQL reads the measure's own SQL, never its name, so a new money
// measure is covered under any name. It is money in each row's currency when
// it reads a minor-unit column and converts nothing to the base currency.
func perRowMoneySQL(expression string) bool {
	return minorUnitColumn.MatchString(expression) && !strings.Contains(expression, reportBaseCurrencyToken)
}

// minorUnitColumn is a column holding an amount in minor units. The schema
// names every such column `*_minor`.
var minorUnitColumn = regexp.MustCompile(`\b[a-z_]+_minor\b`)
