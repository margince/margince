// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The two recurring figures, against the worked examples in the product plan.
//
// Every expected number here is stated as an arithmetic sentence in the case
// name, so a reader can check the figure without running anything. That matters
// more than usual: ARR and committed total are easy to conflate, and a test
// whose expected value was copied from the implementation would agree with a
// conflation rather than catch it.

import (
	"errors"
	"testing"
)

func months(n int) *int { return &n }

func recurringLine(unitPriceMinor int64, interval int, count *int) RecurringLineInput {
	return RecurringLineInput{
		Line: OfferLineInput{
			Quantity: "1", UnitPriceMinor: unitPriceMinor, DiscountPct: "0", TaxRate: "0",
		},
		BillingModel: BillingRecurring, IntervalMonths: months(interval), IntervalCount: count,
	}
}

func oneTimeLine(unitPriceMinor int64) RecurringLineInput {
	return RecurringLineInput{
		Line: OfferLineInput{
			Quantity: "1", UnitPriceMinor: unitPriceMinor, DiscountPct: "0", TaxRate: "0",
		},
		BillingModel: BillingOneTime,
	}
}

func TestOfferRecurringTotals(t *testing.T) {
	t.Parallel()
	four, eight := 4, 8

	for _, c := range []struct {
		name    string
		lines   []RecurringLineInput
		wantARR int64
		wantTCV int64
	}{
		{
			// The plan's first worked example: 1,000 a month for twelve
			// months, plus a 2,000 setup fee.
			name: "monthly for a year plus a setup fee: ARR 12,000, committed 14,000",
			lines: []RecurringLineInput{
				recurringLine(100_000, 1, months(12)),
				oneTimeLine(200_000),
			},
			wantARR: 1_200_000,
			wantTCV: 1_400_000,
		},
		{
			// The plan's second: 3,000 a quarter, four quarters.
			name:    "quarterly for a year: ARR 12,000, committed 12,000",
			lines:   []RecurringLineInput{recurringLine(300_000, 3, months(4))},
			wantARR: 1_200_000,
			wantTCV: 1_200_000,
		},
		{
			// The case that proves the two figures are not the same number.
			// Twice the term is twice the commitment and the SAME annual rate.
			name:    "quarterly for two years: ARR still 12,000, committed 24,000",
			lines:   []RecurringLineInput{recurringLine(300_000, 3, &eight)},
			wantARR: 1_200_000,
			wantTCV: 2_400_000,
		},
		{
			name:    "a term shorter than a year: ARR 12,000, committed 3,000",
			lines:   []RecurringLineInput{recurringLine(300_000, 3, months(1))},
			wantARR: 1_200_000,
			wantTCV: 300_000,
		},
		{
			name:    "half-yearly: ARR is twice the period price",
			lines:   []RecurringLineInput{recurringLine(600_000, 6, months(2))},
			wantARR: 1_200_000,
			wantTCV: 1_200_000,
		},
		{
			name:    "annual: ARR is the period price itself",
			lines:   []RecurringLineInput{recurringLine(1_200_000, 12, months(3))},
			wantARR: 1_200_000,
			wantTCV: 3_600_000,
		},
		{
			// A recurring line nobody has given a term. It has an annual rate
			// — that is a property of the price — but no committed total,
			// because nothing says how long it runs.
			name:    "recurring with no settled term: ARR 12,000, committed nothing",
			lines:   []RecurringLineInput{recurringLine(100_000, 1, nil)},
			wantARR: 1_200_000,
			wantTCV: 0,
		},
		{
			name:    "one-off alone: no ARR at all",
			lines:   []RecurringLineInput{oneTimeLine(500_000)},
			wantARR: 0,
			wantTCV: 500_000,
		},
		{
			// Every line written before the classification existed. Counted
			// toward neither figure: nobody said what this price does, and
			// reading it as one-off would assert an answer.
			name:    "unclassified: counted toward neither figure",
			lines:   []RecurringLineInput{{Line: OfferLineInput{Quantity: "1", UnitPriceMinor: 500_000, DiscountPct: "0", TaxRate: "0"}}},
			wantARR: 0,
			wantTCV: 0,
		},
		{
			name:    "no lines at all",
			lines:   nil,
			wantARR: 0,
			wantTCV: 0,
		},
		{
			// Tax is the government's share, not revenue. A 20% rate must not
			// appear in either figure: 1,000 a month net is 12,000 a year
			// whatever the tax on it.
			name: "20 per cent tax does not reach ARR or the committed net",
			lines: []RecurringLineInput{{
				Line: OfferLineInput{
					Quantity: "1", UnitPriceMinor: 100_000, DiscountPct: "0", TaxRate: "20.00",
				},
				BillingModel: BillingRecurring, IntervalMonths: months(1), IntervalCount: months(12),
			}},
			wantARR: 1_200_000,
			wantTCV: 1_200_000,
		},
		{
			// The discount is part of the net, so it DOES reach both: 1,000 a
			// month less 10% is 900, which is 10,800 a year.
			name: "a discount does reach both figures",
			lines: []RecurringLineInput{{
				Line: OfferLineInput{
					Quantity: "1", UnitPriceMinor: 100_000, DiscountPct: "10.00", TaxRate: "0",
				},
				BillingModel: BillingRecurring, IntervalMonths: months(1), IntervalCount: &four,
			}},
			wantARR: 1_080_000,
			wantTCV: 360_000,
		},
		{
			// Quantity multiplies the period price like any other line.
			name: "three seats at 50 a month: ARR 1,800",
			lines: []RecurringLineInput{{
				Line: OfferLineInput{
					Quantity: "3", UnitPriceMinor: 5_000, DiscountPct: "0", TaxRate: "0",
				},
				BillingModel: BillingRecurring, IntervalMonths: months(1), IntervalCount: months(12),
			}},
			wantARR: 180_000,
			wantTCV: 180_000,
		},
		{
			// A fractional quantity rounds ONCE, in LineTotals, and the annual
			// figure is that rounded net times the periods — never a second
			// rounding of its own.
			name: "half a seat at 33.33 a month rounds once",
			lines: []RecurringLineInput{{
				Line: OfferLineInput{
					Quantity: "0.5", UnitPriceMinor: 3_333, DiscountPct: "0", TaxRate: "0",
				},
				BillingModel: BillingRecurring, IntervalMonths: months(1), IntervalCount: months(12),
			}},
			// 0.5 × 3333 = 1666.5, half up → 1667 a month, × 12 = 20004.
			wantARR: 20_004,
			wantTCV: 20_004,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := OfferRecurringTotals(c.lines)
			if err != nil {
				t.Fatalf("deriving the recurring totals: %v", err)
			}
			if got.ArrMinor != c.wantARR {
				t.Errorf("arr_minor = %d, want %d", got.ArrMinor, c.wantARR)
			}
			if got.NetTcvMinor != c.wantTCV {
				t.Errorf("net_tcv_minor = %d, want %d", got.NetTcvMinor, c.wantTCV)
			}
		})
	}
}

// A recurring line reaching the engine without a cadence is a row the shape
// CHECK refuses, so arriving here means a writer went around it. Refusing beats
// dividing by zero, and beats annualizing at some default that would look like
// an answer.
func TestARecurringLineWithNoCadenceIsRefusedRatherThanGuessed(t *testing.T) {
	t.Parallel()
	_, err := OfferRecurringTotals([]RecurringLineInput{{
		Line:         OfferLineInput{Quantity: "1", UnitPriceMinor: 100_000, DiscountPct: "0", TaxRate: "0"},
		BillingModel: BillingRecurring,
	}})
	if err == nil {
		t.Fatal("a recurring line with no cadence was annualized, which requires inventing a period")
	}
}

// The annualization multiplies, so a price near the ceiling can leave it. It
// must refuse rather than wrap: big.Int.Int64() on an out-of-range value yields
// the low 64 bits, a plausible and frequently negative number the offer row and
// the PDF would both go on to treat as real money.
func TestAnAnnualizedFigurePastTheCeilingIsRefusedRatherThanWrapped(t *testing.T) {
	t.Parallel()
	const nearCeiling = int64(1) << 62
	_, err := OfferRecurringTotals([]RecurringLineInput{
		recurringLine(nearCeiling, 1, months(12)),
	})
	var rangeErr *MoneyRangeError
	if !errors.As(err, &rangeErr) {
		t.Fatalf("annualizing past the ceiling → %v, want a MoneyRangeError", err)
	}
	if rangeErr.Figure != "arr_minor" {
		t.Errorf("refusal names %q, want arr_minor", rangeErr.Figure)
	}
}
