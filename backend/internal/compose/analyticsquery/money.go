// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// The one rule for money kept in each row's own currency: no aggregate folds
// amounts in two currencies into one number. The typed query here and the
// prebuilt report engine in compose both ask it, so they refuse the same plans.

import (
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// PerRowCurrency marks a measure whose every value is in its own row's
// currency, such as a deal's amount_minor.
type PerRowCurrency struct {
	// By is the dimension naming each row's currency.
	By string
	// Base is the measure holding the same amount converted to the
	// installation's base currency, or "" when the population offers none.
	Base string
}

// foldsValues are the aggregates that combine several values into one.
// A count of a money field is how many rows hold an amount, which means the
// same in every currency, so the counts are absent.
var foldsValues = map[AggFn]bool{Sum: true, Avg: true, Min: true, Max: true, Median: true, P75: true}

// Mixes says whether fn over this measure would combine amounts in different
// currencies. The rule is static: a plan is safe when it groups by the
// currency or pins one with an equality filter, whatever the data holds. A
// check against the rows would let a saved question answer today and refuse
// tomorrow, after somebody priced one deal in yen.
//
//craft:ignore naked-any a filter value is schemaless JSON on both engines
func (c PerRowCurrency) Mixes(fn AggFn, groupBy []string, pinned any) bool {
	if !foldsValues[fn] || slices.Contains(groupBy, c.By) {
		return false
	}
	currency, ok := pinned.(string)
	return !ok || currency == ""
}

// Refusal says why the plan was refused and what to ask instead.
func (c PerRowCurrency) Refusal(fn AggFn, field string) *RefusalError {
	suggest := fmt.Sprintf("group by %s or filter %s to one currency", c.By, c.By)
	if c.Base != "" {
		suggest += fmt.Sprintf(", or measure %s, which is converted to the base currency", c.Base)
	}
	return &RefusalError{
		Kind: RefusalInvalid,
		Message: fmt.Sprintf("%s of %s would add amounts in different currencies into one number with no unit",
			fn, httperr.QuoteCaller(field)),
		Suggest: suggest,
	}
}

// pinnedValue is the value an equality filter on field fixes, or nil.
//
//craft:ignore naked-any a filter value is schemaless JSON
func pinnedValue(filters []Filter, field string) any {
	for _, f := range filters {
		if f.Field == field && f.Op == OpEq {
			return f.Value
		}
	}
	return nil
}

// validateCurrencyBasis refuses a measure that would fold several currencies.
func validateCurrencyBasis(entity Entity, q Query) error {
	for _, m := range q.Measures {
		money := entity.Fields[m.Field].Currency
		if money == nil {
			continue
		}
		if money.Mixes(m.Fn, q.GroupBy, pinnedValue(q.Filters, money.By)) {
			return money.Refusal(m.Fn, m.Field)
		}
	}
	return nil
}
