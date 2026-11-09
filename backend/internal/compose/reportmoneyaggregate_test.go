// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
)

// Each aggregate over a converted amount says what it can price, and no more.
// A sum reads an empty group as zero. An average, a minimum and a maximum have
// no zero, so one unpriced amount leaves them without an answer. A count is not
// money, and an amount with no coverage column converts nothing.
func TestEachMoneyAggregateSaysOnlyWhatItCanPrice(t *testing.T) {
	const amount, coverage = "v.base", "v.unpriced"
	spec := reportSpec{sumCoverage: map[string]string{"value": coverage}}
	unpriced := "sum(" + coverage + ")"

	for fn, want := range map[string]string{
		aggFnSum:   deals.FinalizeBaseTotalSQL("sum(v.base)", unpriced),
		aggFnAvg:   deals.StrictBaseTotalSQL("avg(v.base)", unpriced),
		aggFnMin:   deals.StrictBaseTotalSQL("min(v.base)", unpriced),
		aggFnMax:   deals.StrictBaseTotalSQL("max(v.base)", unpriced),
		aggFnCount: "count(v.base)",
	} {
		if got := reportMoneyAggregateSQL(spec, reportAggregate{Fn: fn, Field: "value"}, amount); got != want {
			t.Errorf("%s over a converted amount renders %q, want %q", fn, got, want)
		}
	}

	if got := reportMoneyAggregateSQL(spec, reportAggregate{Fn: aggFnAvg, Field: "probability"}, "v.p"); got != "avg(v.p)" {
		t.Errorf("an average over a field with no coverage renders %q, want the plain average", got)
	}
}
