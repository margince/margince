// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"errors"
	"strings"
	"testing"
)

func moneySchema(base string) Schema {
	return Schema{Entities: map[string]Entity{"deals": {
		Name: "deals",
		Fields: map[string]Field{
			"currency":          {Name: "currency", Expr: "t.currency", Kind: KindDimension},
			"stage_id":          {Name: "stage_id", Expr: "t.stage_id", Kind: KindDimension},
			"amount_base_minor": {Name: "amount_base_minor", Expr: "base(t)", Kind: KindMeasure, Shape: ShapeNumber},
			"amount_minor": {
				Name: "amount_minor", Expr: "t.amount_minor", Kind: KindMeasure, Shape: ShapeNumber,
				Currency: &PerRowCurrency{By: "currency", Base: base},
			},
		},
	}}}
}

func TestEveryAggregateThatFoldsAmountsIsRefusedAcrossCurrencies(t *testing.T) {
	t.Parallel()
	for _, fn := range []AggFn{Sum, Avg, Min, Max, Median, P75} {
		q := Query{Entity: "deals", GroupBy: []string{"stage_id"}, Measures: []Measure{{Fn: fn, Field: "amount_minor"}}}
		var refusal *RefusalError
		if err := q.Validate(moneySchema("amount_base_minor")); !errors.As(err, &refusal) {
			t.Errorf("%s of amount_minor by stage alone answered %v, want a refusal", fn, err)
			continue
		}
		if refusal.Kind != RefusalInvalid {
			t.Errorf("%s: refusal kind %q, want invalid", fn, refusal.Kind)
		}
		for _, repair := range []string{"group by currency", "filter currency", "amount_base_minor"} {
			if !strings.Contains(refusal.Suggest, repair) {
				t.Errorf("%s: the suggestion %q does not offer %q", fn, refusal.Suggest, repair)
			}
		}
	}
}

func TestAPlanThatHoldsOneCurrencyPerRowIsAnswered(t *testing.T) {
	t.Parallel()
	sum := []Measure{{Fn: Sum, Field: "amount_minor"}}
	for name, q := range map[string]Query{
		"grouped by currency": {Entity: "deals", GroupBy: []string{"stage_id", "currency"}, Measures: sum},
		"pinned to one":       {Entity: "deals", Measures: sum, Filters: []Filter{{Field: "currency", Op: OpEq, Value: "JPY"}}},
		"counted, not folded": {Entity: "deals", Measures: []Measure{{Fn: CountDistinct, Field: "amount_minor"}}},
		"converted measure":   {Entity: "deals", Measures: []Measure{{Fn: Sum, Field: "amount_base_minor"}}},
	} {
		if err := q.Validate(moneySchema("amount_base_minor")); err != nil {
			t.Errorf("%s: refused with %v", name, err)
		}
	}
}

// Only an equality pins a currency. Excluding one still leaves several.
func TestAFilterThatLeavesSeveralCurrenciesDoesNotPin(t *testing.T) {
	t.Parallel()
	sum := []Measure{{Fn: Sum, Field: "amount_minor"}}
	for name, filter := range map[string]Filter{
		"ne":          {Field: "currency", Op: OpNe, Value: "EUR"},
		"is_not_null": {Field: "currency", Op: OpIsNotNull},
		"empty value": {Field: "currency", Op: OpEq, Value: ""},
	} {
		q := Query{Entity: "deals", Measures: sum, Filters: []Filter{filter}}
		if err := q.Validate(moneySchema("")); err == nil {
			t.Errorf("a %s filter on currency was taken as one currency", name)
		}
	}
}

func TestTheRefusalNamesNoConvertedMeasureThePopulationLacks(t *testing.T) {
	t.Parallel()
	q := Query{Entity: "deals", Measures: []Measure{{Fn: Sum, Field: "amount_minor"}}}
	var refusal *RefusalError
	if !errors.As(q.Validate(moneySchema("")), &refusal) {
		t.Fatal("the ungrouped sum was answered")
	}
	if strings.Contains(refusal.Suggest, "converted") {
		t.Errorf("the suggestion offers a converted measure this population does not have: %q", refusal.Suggest)
	}
}

// The vocabulary check alone admits the plan, which is what lets a saved
// question that now mixes currencies stay listed.
func TestTheVocabularyCheckLeavesTheCurrencyRuleToValidate(t *testing.T) {
	t.Parallel()
	q := Query{Entity: "deals", Measures: []Measure{{Fn: Sum, Field: "amount_minor"}}}
	if err := q.ValidateVocabulary(moneySchema("")); err != nil {
		t.Errorf("every name in the plan exists, and the vocabulary check refused it: %v", err)
	}
}
