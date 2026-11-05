// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"errors"
	"math"
	"slices"
	"testing"
)

func pricedDealLine(quantity string, priceMinor int64, billing string) DealLineInput {
	model := billing
	return DealLineInput{Quantity: &quantity, UnitPriceMinor: &priceMinor, DiscountPct: zeroPct, TaxRate: new(zeroPct), BillingModel: &model}
}

func recurringDealLine(quantity string, priceMinor int64, months int, count *int) DealLineInput {
	line := pricedDealLine(quantity, priceMinor, BillingRecurring)
	line.IntervalMonths = &months
	line.IntervalCount = count
	return line
}

func mustTotals(t *testing.T, lines ...DealLineInput) DealCommercialFigures {
	t.Helper()
	figures, err := DealCommercialTotals(lines)
	if err != nil {
		t.Fatalf("DealCommercialTotals: %v", err)
	}
	return figures
}

func wantMinor(t *testing.T, name string, got *int64, want int64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %d", name, got, want)
	}
}

func TestAHundredBananasAtTwoEurosAreWorthTwoHundredNet(t *testing.T) {
	figures := mustTotals(t, pricedDealLine("100", 200, BillingOneTime))
	wantMinor(t, "committed net", figures.CommittedNetMinor, 20000)
	wantMinor(t, "annual recurring", figures.ArrMinor, 0)
	if len(figures.Missing) != 0 {
		t.Fatalf("missing = %v, want none", figures.Missing)
	}
}

func TestADiscountedTaxedLineKeepsTheHeadlineNet(t *testing.T) {
	line := pricedDealLine("150", 200, BillingOneTime)
	line.DiscountPct = "10"
	line.TaxRate = new("20")
	figures := mustTotals(t, line)
	wantMinor(t, "committed net", figures.CommittedNetMinor, 27000)
	wantMinor(t, "tax", figures.TaxMinor, 5400)
	wantMinor(t, "gross", figures.GrossMinor, 32400)
}

func TestAnUnknownPriceLeavesTheValueOpenAndKeepsTheKnownPart(t *testing.T) {
	unpriced := DealLineInput{Quantity: new("5"), DiscountPct: zeroPct, TaxRate: new(zeroPct), BillingModel: new(BillingOneTime)}
	figures := mustTotals(t, pricedDealLine("100", 200, BillingOneTime), unpriced)
	if figures.CommittedNetMinor != nil {
		t.Fatalf("committed net = %d, want unknown", *figures.CommittedNetMinor)
	}
	if figures.KnownCommittedNetMinor != 20000 {
		t.Fatalf("known part = %d, want 20000", figures.KnownCommittedNetMinor)
	}
	if !slices.Contains(figures.Missing, missingUnitPrice) {
		t.Fatalf("missing = %v, want unit_price", figures.Missing)
	}
}

func TestAnUnclassifiedLineLeavesTheValueOpen(t *testing.T) {
	line := pricedDealLine("1", 100, BillingOneTime)
	line.BillingModel = nil
	figures := mustTotals(t, line)
	if figures.CommittedNetMinor != nil || figures.ArrMinor != nil {
		t.Fatalf("figures = %+v, want committed net and ARR unknown", figures)
	}
	if !slices.Contains(figures.Missing, missingBillingModel) {
		t.Fatalf("missing = %v, want billing_model", figures.Missing)
	}
}

func TestAFreeItemIsWorthZeroAndIsComplete(t *testing.T) {
	figures := mustTotals(t, pricedDealLine("3", 0, BillingOneTime))
	wantMinor(t, "committed net", figures.CommittedNetMinor, 0)
}

func TestARecurringLineCommitsItsPeriodsAndAnnualisesSeparately(t *testing.T) {
	twelve := 12
	figures := mustTotals(t, recurringDealLine("1", 5000, 1, &twelve), pricedDealLine("1", 10000, BillingOneTime))
	wantMinor(t, "committed net", figures.CommittedNetMinor, 70000)
	wantMinor(t, "annual recurring", figures.ArrMinor, 60000)
}

func TestARecurringLineWithNoTermHasAnnualValueButNoCommittedTotal(t *testing.T) {
	figures := mustTotals(t, recurringDealLine("1", 5000, 1, nil))
	wantMinor(t, "annual recurring", figures.ArrMinor, 60000)
	if figures.CommittedNetMinor != nil {
		t.Fatalf("committed net = %d, want unknown", *figures.CommittedNetMinor)
	}
	if !slices.Contains(figures.Missing, missingIntervalCount) {
		t.Fatalf("missing = %v, want interval_count", figures.Missing)
	}
}

func TestADealWithNoLineHasNoValue(t *testing.T) {
	figures := mustTotals(t)
	if figures.CommittedNetMinor != nil || figures.ArrMinor != nil {
		t.Fatalf("figures = %+v, want no value", figures)
	}
	if !slices.Equal(figures.Missing, []string{missingLines}) {
		t.Fatalf("missing = %v, want lines", figures.Missing)
	}
}

func TestAFractionalQuantityRoundsOnceOnTheLine(t *testing.T) {
	figures := mustTotals(t, pricedDealLine("0.333", 100, BillingOneTime), pricedDealLine("0.333", 100, BillingOneTime))
	// Each line is 33.3 minor units, rounded to 33 before the sum.
	wantMinor(t, "committed net", figures.CommittedNetMinor, 66)
}

func TestAValueTooLargeToStoreIsRefused(t *testing.T) {
	big := int64(math.MaxInt64 / 2)
	_, err := DealCommercialTotals([]DealLineInput{pricedDealLine("3", big, BillingOneTime)})
	if _, ok := errors.AsType[*MoneyRangeError](err); !ok {
		t.Fatalf("err = %v, want MoneyRangeError", err)
	}
}
