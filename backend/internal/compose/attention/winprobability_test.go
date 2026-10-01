// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// withStagedDeal is a priced deal whose stage records a win probability.
func withStagedDeal(amount CurrencyAmount, winProbability int) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) {
		minor, currency, prob := amount.Minor, amount.Currency, winProbability
		i.Deal = &crmcontracts.AttentionDealFacts{
			AmountMinor: &minor, Currency: &currency, WinProbability: &prob,
		}
	}
}

// The contract declares `win_probability` on the deal a card names, and the
// projection has to send it. Declared and never assigned, it reaches every
// client absent, so a reader who wants the stage's odds beside the amount goes
// and reads the stage themselves — a second read of a fact this row already
// held, which is the round trip these deal facts exist to save.
//
// It is carried, NOT multiplied. The contract says `expected_minor_base` is the
// deal's own money and this is a property of the stage it sits in, so the test
// asserts the amount is untouched alongside: a projection that started
// weighting would satisfy an assertion about the probability alone.
func TestTheDealFactsCarryTheStagesWinProbability(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("yen", "deal_at_risk", withStagedDeal(fiveMillionYen, 40))),
	}
	fx := stubFX{base: "EUR", answers: map[CurrencyAmount]int64{fiveMillionYen: 3_000_000}}

	out := pricedWorklist(t, fx, day)

	deal := out.Queue[0].Deal
	if deal == nil {
		t.Fatal("the row carries no deal facts at all")
	}
	if deal.WinProbability == nil {
		t.Fatal("win_probability is absent — the contract declares it on this row, so a client " +
			"reads null and goes to the stage for a fact this row already had in hand")
	}
	if *deal.WinProbability != 40 {
		t.Errorf("win_probability = %d, want 40", *deal.WinProbability)
	}
	if deal.ExpectedMinorBase == nil || *deal.ExpectedMinorBase != 3_000_000 {
		t.Errorf("expected_minor_base = %v, want 3000000 unweighted — the probability is a fact "+
			"beside the money, and multiplying them here would print a risk-adjusted figure the "+
			"contract says this API does not compute", deal.ExpectedMinorBase)
	}
}

// A row whose deal facts carry no probability sends none, rather than nought.
//
// Zero is a real score — a lost stage carries exactly 0 — so a projection that
// defaulted the missing case to 0 would tell a reader a deal is hopeless where
// the lane simply never supplied the fact.
func TestARowWithNoProbabilityInItsFactsSendsNoneRatherThanNought(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("yen", "deal_at_risk", withPricedDeal(fiveMillionYen))),
	}
	fx := stubFX{base: "EUR", answers: map[CurrencyAmount]int64{fiveMillionYen: 3_000_000}}

	out := pricedWorklist(t, fx, day)

	if deal := out.Queue[0].Deal; deal != nil && deal.WinProbability != nil {
		t.Errorf("win_probability = %d on a deal whose stage recorded none", *deal.WinProbability)
	}
}
