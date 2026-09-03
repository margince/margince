// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The ordering compares money in ONE currency.
//
// The defect these tests hold the door against: expected revenue compared as
// raw minor units, which ranks a yen integer against a euro one and gets the
// answer wrong in whichever direction the two scales happen to fall. Every deal
// in the demo data is EUR, which is why nothing ever looked wrong — a
// single-currency pipeline is correct by coincidence, and these fixtures are
// deliberately not one.

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// stubFX answers a conversion the fixtures state outright, keyed by the
// amount's own minor units and currency.
//
// It does NOT recompute the arithmetic. Rate handling and the two currencies'
// minor-unit scales belong to deals.ConvertToBase, which proves them against
// its own cases (deals/fxconvert_test.go) and against real rows in the
// integration lane. A stub that multiplied here would be a second, simpler
// implementation of that arithmetic, free to be wrong in the same direction as
// a bug and hide it — which is exactly what a milli-rate version of this stub
// did: it restated a bare minor × rate and made a ¥5,000,000 deal look like
// €300.
type stubFX struct {
	base string
	// answers maps a deal's own {minor, currency} to what it is worth in the
	// base currency's minor units. An amount absent from the map is one the
	// estate cannot price.
	answers map[CurrencyAmount]int64
	err     error
}

func (f stubFX) ToBase(_ context.Context, _ time.Time, amounts []CurrencyAmount) ([]*int64, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	out := make([]*int64, len(amounts))
	for i, amount := range amounts {
		if converted, priced := f.answers[amount]; priced {
			minor := converted
			out[i] = &minor
		}
	}
	return out, f.base, nil
}

// eur is one amount in the base currency, which converts to itself.
func eur(minor int64) CurrencyAmount { return CurrencyAmount{Minor: minor, Currency: "EUR"} }

// fiveMillionYen is the cross-scale fixture: ¥5,000,000, worth about €30,000.
//
// JPY carries no minor unit where EUR carries two, so this one amount is both
// the largest figure on every page below and — as a bare integer against euro
// minor units — the one a wrong conversion misreads by a hundredfold. A
// same-scale pair could not tell the two readings apart.
var fiveMillionYen = CurrencyAmount{Minor: 5_000_000, Currency: "JPY"}

func withPricedDeal(amount CurrencyAmount) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) {
		minor, currency := amount.Minor, amount.Currency
		i.Deal = &crmcontracts.AttentionDealFacts{AmountMinor: &minor, Currency: &currency}
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
	return reader.worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, leadRead{}, worklistCursor{}, nil)
}

// The ordering compares what deals are WORTH, not what their integers say.
//
// The yen deal is ¥5,000,000 — five million yen, about €30,000, the largest on
// the page by worth AND by integer, so on its own it proves nothing. The one
// that does is `eur-20k`: €20,000, the second largest by worth and the second
// SMALLEST integer of the four. Raw minor units rank it third from bottom;
// converted, it comes second. That inversion is the assertion.
//
// Four deals rather than three so the median leaves two of them material and
// the revenue step is what separates the top pair. With three, only the yen
// deal cleared the bar and a LEVEL difference decided the leading pair, which
// is a different rule proving itself.
func TestTheOrderingComparesWhatDealsAreWorth(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("yen", "deal_at_risk", withPricedDeal(fiveMillionYen)),
			item("eur-20k", "deal_at_risk", withPricedDeal(eur(2_000_000))),
			item("eur-4k", "deal_at_risk", withPricedDeal(eur(400_000))),
			item("eur-1k", "deal_at_risk", withPricedDeal(eur(100_000))),
		),
	}
	fx := stubFX{base: "EUR", answers: map[CurrencyAmount]int64{
		// ¥5,000,000 at 1 JPY = 0.006 EUR is €30,000.
		fiveMillionYen: 3_000_000,
		eur(2_000_000): 2_000_000,
		eur(400_000):   400_000,
		eur(100_000):   100_000,
	}}

	out := pricedWorklist(t, fx, day)

	assertOrder(t, out.Queue, "yen", "eur-20k", "eur-4k", "eur-1k")
	// The leading row explains itself against the second in the figures the
	// ordering used, and names the currency they are genuinely in.
	comparison := out.Queue[0].AboveNext
	if comparison == nil || comparison.Comparator != crmcontracts.WorklistComparisonComparatorExpectedRevenue {
		t.Fatalf("the leading deal explains itself with %+v, wanted expected_revenue", comparison)
	}
	if comparison.Mine == nil || comparison.Mine.Minor == nil || *comparison.Mine.Minor != 3_000_000 {
		t.Fatalf("mine = %+v, wanted the yen deal's €30,000 as 3000000 EUR minor units", comparison.Mine)
	}
	if comparison.Theirs == nil || comparison.Theirs.Minor == nil || *comparison.Theirs.Minor != 2_000_000 {
		t.Fatalf("theirs = %+v, wanted 2000000", comparison.Theirs)
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
			item("yen", "deal_at_risk", withPricedDeal(fiveMillionYen)),
			item("eur-60k", "deal_at_risk", withPricedDeal(eur(6_000_000))),
			item("eur-1k", "deal_at_risk", withPricedDeal(eur(100_000))),
		),
	}
	fx := stubFX{base: "EUR", answers: map[CurrencyAmount]int64{
		fiveMillionYen: 3_000_000, // ¥5,000,000 is €30,000
		eur(6_000_000): 6_000_000,
		eur(100_000):   100_000,
	}}

	out := pricedWorklist(t, fx, day)

	// Converted the set is €1,000 / €30,000 / €60,000 and the median is the
	// YEN deal, €30,000. Raw it is 100,000 / 5,000,000 / 6,000,000 and the
	// median is the same integer 5,000,000 — read as €50,000, which is a bar
	// no deal on this page is actually worth. So the number below distinguishes
	// a converted median from a raw one, rather than agreeing by coincidence.
	if out.Summary.MaterialThresholdMinor == nil || *out.Summary.MaterialThresholdMinor != 3_000_000 {
		t.Fatalf("the bar is %v, wanted the converted median 3000000 (€30,000); "+
			"5000000 is the raw yen integer read as euros", out.Summary.MaterialThresholdMinor)
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
			item("no-rate", "deal_at_risk", withPricedDeal(CurrencyAmount{Minor: 9_000_000, Currency: "GBP"})),
			item("priced", "deal_at_risk", withPricedDeal(eur(500_000))),
			item("unitless", "deal_at_risk", withDeal(7_000_000)),
		),
	}
	// The estate prices the euro deal and holds no GBP rate at all.
	fx := stubFX{base: "EUR", answers: map[CurrencyAmount]int64{eur(500_000): 500_000}}

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
	if out.Summary.MaterialThresholdMinor == nil || *out.Summary.MaterialThresholdMinor != 500_000 {
		t.Fatalf("the bar is %v, wanted 500000 from the only priced deal", *out.Summary.MaterialThresholdMinor)
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

	out := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, leadRead{}, worklistCursor{}, nil)

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
		AtRisk: lane(item("d", "deal_at_risk", withPricedDeal(eur(100_000)))),
	}
	s := &Service{fx: stubFX{err: errors.New("the rate read broke")}}

	if _, err := s.priceTheDay(t.Context(), day); err == nil {
		t.Fatal("a broken rate read priced the day anyway")
	}
}

// A day whose at-risk deals all lack a currency has NOTHING to convert, and
// that is not the same as having no conversion.
//
// The bound seam finds no convertible amount and asks the estate nothing. If
// that answered the same zero dayMoney an UNBOUND seam answers, the ordering
// would fall back to raw minor units and rank a yen integer against a euro one
// — under a seam that was bound precisely to stop it. The deals are unpriced;
// the day is still converted.
func TestADayOfUnitlessDealsIsConvertedWithNothingPriced(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("big-integer", "deal_at_risk", withDeal(5_000_000)),
			item("small-integer", "deal_at_risk", withDeal(100_000)),
		),
	}
	fx := stubFX{base: "EUR", answers: map[CurrencyAmount]int64{}}

	out := pricedWorklist(t, fx, day)

	// No money verdict may be stated over a figure nothing converted.
	for _, row := range out.Queue {
		for _, because := range row.Because {
			if because.Kind == "material" || because.Kind == "below_material" {
				t.Fatalf("%q states %q over an amount with no currency", row.Id, because.Kind)
			}
		}
	}
	// And no threshold, because there was no comparable figure to take one from.
	if out.Summary.MaterialThresholdMinor != nil {
		t.Fatalf("a bar of %d was taken from amounts in no currency", *out.Summary.MaterialThresholdMinor)
	}
}

// The contract promises a per-row base-currency figure
// (WorklistDealFacts.ExpectedMinorBase) and the projection must actually send
// one: a client wanting to show or total the converted amount otherwise finds
// null and either shows nothing or re-derives the conversion itself, a second
// answer to "what is this deal worth here" of exactly the kind this seam
// exists to prevent.
func TestTheDealFactsCarryTheConvertedExpectedRevenue(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("yen", "deal_at_risk", withPricedDeal(fiveMillionYen))),
	}
	fx := stubFX{base: "EUR", answers: map[CurrencyAmount]int64{fiveMillionYen: 3_000_000}}

	out := pricedWorklist(t, fx, day)

	deal := out.Queue[0].Deal
	if deal == nil {
		t.Fatal("the row carries no deal facts at all")
	}
	if deal.ExpectedMinorBase == nil || *deal.ExpectedMinorBase != 3_000_000 {
		t.Fatalf("expected_minor_base = %v, wanted 3000000 (the converted €30,000)", deal.ExpectedMinorBase)
	}
}

// A deal the estate cannot price states no base-currency figure: null means
// "could not be priced", the same null a client already reads for an unpriced
// deal's other money fields, not a second and different meaning of null.
func TestAnUnpricedDealCarriesNoExpectedMinorBase(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("no-rate", "deal_at_risk", withPricedDeal(CurrencyAmount{Minor: 9_000_000, Currency: "GBP"}))),
	}
	fx := stubFX{base: "EUR", answers: map[CurrencyAmount]int64{}}

	out := pricedWorklist(t, fx, day)

	if deal := out.Queue[0].Deal; deal != nil && deal.ExpectedMinorBase != nil {
		t.Fatalf("expected_minor_base = %v on a deal the estate holds no rate for", *deal.ExpectedMinorBase)
	}
}

// Without the FX seam bound there is no base currency at all, so the raw
// amount must never be sent AS IF it were a base-currency figure — that would
// silently misprice every currency but the base's own.
func TestAnUnboundSeamCarriesNoExpectedMinorBase(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("d", "deal_at_risk", withDeal(40_000))),
	}

	out := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, leadRead{}, worklistCursor{}, nil)

	if deal := out.Queue[0].Deal; deal != nil && deal.ExpectedMinorBase != nil {
		t.Fatalf("expected_minor_base = %v with no FX seam bound at all", *deal.ExpectedMinorBase)
	}
}
