// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
)

// Every aggregate the report engine offers says only what it can price over a
// converted amount. The functions come from the engine's own vocabulary, so one
// added later is held to the rule without anyone remembering to list it.
//
// A sum keeps what it priced. A count is not money. Every other function has no
// answer for a group holding an amount that could not be priced, and folds each
// row as that row reads.
func TestEveryAggregateOverAConvertedAmountSaysOnlyWhatItCanPrice(t *testing.T) {
	const amount, coverage = "v.base", "v.unpriced"
	spec := reportSpec{
		measures:    map[string]string{"value": amount},
		sumCoverage: map[string]string{"value": coverage},
	}
	strict := deals.StrictBaseTotalSQL("", "sum("+coverage+")")
	strictHead := strict[:strings.Index(strict, "ELSE")]
	row := deals.FinalizeBaseTotalSQL(amount, coverage)

	for _, fn := range reportPlanVocabulary().Functions {
		_, sel, err := aggregateSelect(spec, reportAggregate{Fn: fn, Field: "value", As: "out"})
		if err != nil {
			t.Fatalf("%s over a converted amount: %v", fn, err)
		}
		switch fn {
		case aggFnCount:
			if strings.Contains(sel, coverage) {
				t.Errorf("count renders %q, want a count that reads no coverage", sel)
			}
		case aggFnSum:
			if want := deals.FinalizeBaseTotalSQL("sum("+amount+")", "sum("+coverage+")"); !strings.HasPrefix(sel, want) {
				t.Errorf("sum renders %q, want the finalized total %q", sel, want)
			}
		default:
			if !strings.HasPrefix(sel, strictHead) || !strings.Contains(sel, row) {
				t.Errorf("%s renders %q, want the strict form over each row as it reads", fn, sel)
			}
		}
	}
}

// A measure with no coverage is not a converted amount, and is aggregated as is.
func TestAnAggregateOverAnUncoveredMeasureIsLeftAsIs(t *testing.T) {
	spec := reportSpec{measures: map[string]string{"age": "v.age"}}
	if got := reportMoneyAggregateSQL(spec, reportAggregate{Fn: aggFnAvg, Field: "age"}, "v.age"); got != "avg(v.age)" {
		t.Errorf("an average over an uncovered measure renders %q, want avg(v.age)", got)
	}
}
