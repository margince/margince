// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The ordering compares money in ONE currency.
//
// The defect these tests hold the door against: expected revenue compared as
// raw minor units, so ¥8,000,000 outranked €40,000 by being the larger integer
// while being worth a fraction of it. Every deal in the demo data is EUR, which
// is why nothing ever looked wrong — a single-currency pipeline is correct by
// coincidence, and these fixtures are deliberately not one.

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// stubFX converts by a per-currency rate in thousandths, the way a stored rate
// would, and answers nil for a currency it holds no rate for.
type stubFX struct {
	base  string
	milli map[string]int64
	err   error
}

func (f stubFX) ToBase(_ context.Context, _ time.Time, amounts []CurrencyAmount) ([]*int64, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	out := make([]*int64, len(amounts))
	for i, amount := range amounts {
		if amount.Currency == f.base {
			minor := amount.Minor
			out[i] = &minor
			continue
		}
		rate, held := f.milli[amount.Currency]
		if !held {
			continue
		}
		minor := amount.Minor * rate / 1000
		out[i] = &minor
	}
	return out, f.base, nil
}

func withPricedDeal(minor int64, currency string) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) {
		amount := minor
		i.Deal = &crmcontracts.AttentionDealFacts{AmountMinor: &amount, Currency: &currency}
	}
}

// pricedWorklist runs the projection the way Worklist does once the day is
// assembled: price first, then project through the same copy.
func pricedWorklist(t *testing.T, fx BaseMoney, day crmcontracts.Attention) crmcontracts.Worklist {
	t.Helper()
	reader := &Service{fx: fx}
	money, err := reader.priceTheDay(t.Context(), day)
	if err != nil {
		t.Fatalf("pricing the day: %v", err)
	}
	reader.money = money
	return reader.worklistFrom(t.Context(), day, scopeAll, "", 25, nil, leadRead{})
}

// A converted euro deal outranks a yen deal whose raw integer is larger. This
// is the ordering the raw comparison got exactly backwards.
func TestALargerRawYenFigureDoesNotOutrankAEuroDeal(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("yen", "deal_at_risk", withPricedDeal(5_000_000, "JPY")),
			item("big-eur", "deal_at_risk", withPricedDeal(40_000, "EUR")),
			item("mid-eur", "deal_at_risk", withPricedDeal(20_000, "EUR")),
			item("small-eur", "deal_at_risk", withPricedDeal(10_000, "EUR")),
		),
	}
	// ¥5,000,000 × 0.006 = 30,000 base units: material, and still below big-eur.
	fx := stubFX{base: "EUR", milli: map[string]int64{"JPY": 6}}

	out := pricedWorklist(t, fx, day)

	assertOrder(t, out.Queue, "big-eur", "yen", "mid-eur", "small-eur")
	// The first row explains itself against the second in the figures the
	// ordering used, and names the currency they are genuinely in.
	comparison := out.Queue[0].AboveNext
	if comparison == nil || comparison.Comparator != crmcontracts.WorklistComparisonComparatorExpectedRevenue {
		t.Fatalf("the leading deal explains itself with %+v, wanted expected_revenue", comparison)
	}
	if comparison.Mine == nil || comparison.Mine.Minor == nil || *comparison.Mine.Minor != 40_000 {
		t.Fatalf("mine = %+v, wanted the converted 40000", comparison.Mine)
	}
	if comparison.Theirs == nil || comparison.Theirs.Minor == nil || *comparison.Theirs.Minor != 30_000 {
		t.Fatalf("theirs = %+v, wanted the converted 30000, not the raw 8000000", comparison.Theirs)
	}
	if comparison.Mine.Currency == nil || *comparison.Mine.Currency != "EUR" {
		t.Fatalf("the compared figure claims currency %v, wanted the base EUR", comparison.Mine.Currency)
	}
}

// The material bar is the median of CONVERTED figures, and the summary names
// the currency it is stated in — the promise base_currency has carried unsent.
func TestTheMaterialBarIsTakenFromConvertedFiguresAndNamesItsCurrency(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("yen", "deal_at_risk", withPricedDeal(5_000_000, "JPY")),
			item("yen2", "deal_at_risk", withPricedDeal(6_000_000, "JPY")),
			item("big-eur", "deal_at_risk", withPricedDeal(40_000, "EUR")),
			item("mid-eur", "deal_at_risk", withPricedDeal(20_000, "EUR")),
			item("small-eur", "deal_at_risk", withPricedDeal(10_000, "EUR")),
		),
	}
	fx := stubFX{base: "EUR", milli: map[string]int64{"JPY": 6}}

	out := pricedWorklist(t, fx, day)

	// Converted figures 10k, 20k, 30k, 36k, 40k: the median is 30k. A median
	// over raw amounts is 40k, with both yen integers above every euro one.
	if out.Summary.MaterialThresholdMinor == nil || *out.Summary.MaterialThresholdMinor != 30_000 {
		t.Fatalf("the bar is %v, wanted the converted median 30000", out.Summary.MaterialThresholdMinor)
	}
	if out.Summary.BaseCurrency == nil || *out.Summary.BaseCurrency != "EUR" {
		t.Fatalf("the summary names %v, wanted EUR — a threshold without units cannot be checked", out.Summary.BaseCurrency)
	}
}

// A deal the estate holds no rate for ranks as unpriced: below every priced
// deal, with no material verdict — never by its raw number in the wrong units.
func TestADealWithNoStoredRateRanksAsUnpriced(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("no-rate", "deal_at_risk", withPricedDeal(9_000_000, "GBP")),
			item("priced", "deal_at_risk", withPricedDeal(5_000, "EUR")),
			item("unitless", "deal_at_risk", withDeal(7_000_000)),
		),
	}
	fx := stubFX{base: "EUR", milli: map[string]int64{}}

	out := pricedWorklist(t, fx, day)

	assertOrder(t, out.Queue, "priced", "no-rate", "unitless")
	for _, row := range out.Queue {
		if row.Id == "priced" {
			continue
		}
		for _, because := range row.Because {
			if because.Kind == "material" || because.Kind == "below_material" {
				t.Fatalf("%q states a money verdict %q over a figure that was never converted", row.Id, because.Kind)
			}
		}
	}
	// The bar comes from the one priced deal, not from the two raw integers.
	if out.Summary.MaterialThresholdMinor == nil || *out.Summary.MaterialThresholdMinor != 5_000 {
		t.Fatalf("the bar is %v, wanted 5000 from the only priced deal", out.Summary.MaterialThresholdMinor)
	}
}

// Without the seam the figures are raw minor units in no one currency, and the
// summary must not claim otherwise.
func TestAnUnboundSeamNamesNoBaseCurrency(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("a", "deal_at_risk", withDeal(40_000)),
			item("b", "deal_at_risk", withDeal(20_000)),
		),
	}

	out := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, nil, leadRead{})

	if out.Summary.MaterialThresholdMinor == nil {
		t.Fatal("raw amounts still take a median; the bar should stand")
	}
	if out.Summary.BaseCurrency != nil {
		t.Fatalf("an unconverted bar claims to be in %q", *out.Summary.BaseCurrency)
	}
	comparison := out.Queue[0].AboveNext
	if comparison != nil && comparison.Mine != nil && comparison.Mine.Currency != nil {
		t.Fatalf("an unconverted figure claims currency %q", *comparison.Mine.Currency)
	}
}

// A conversion that cannot be read fails the read. Ranking on raw
// cross-currency numbers is the defect, not a fallback.
func TestAFailedConversionFailsThePricing(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("d", "deal_at_risk", withPricedDeal(1_000, "EUR"))),
	}
	s := &Service{fx: stubFX{err: errors.New("the rate read broke")}}

	if _, err := s.priceTheDay(t.Context(), day); err == nil {
		t.Fatal("a broken rate read priced the day anyway")
	}
}
