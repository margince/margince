// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import "testing"

func TestABudgetWithNoCurrencyIsReadInTheFallbackAndMarkedAsAGuess(t *testing.T) {
	t.Parallel()
	for quote, want := range map[string]int64{
		"our budget is 10,000 per year": 1_000_000, "budget around 25k": 2_500_000, "we have 8.500 set aside": 850_000,
	} {
		minor, currency, assumed, ok := QuotedBudget(quote, "EUR")
		if !ok || minor != want || currency != "EUR" || !assumed {
			t.Errorf("QuotedBudget(%q, EUR) = %d %s assumed=%v ok=%v, want %d EUR assumed", quote, minor, currency, assumed, ok, want)
		}
	}
}

func TestABudgetThatNamesItsCurrencyIsReadAsStated(t *testing.T) {
	t.Parallel()
	minor, currency, assumed, ok := QuotedBudget("a budget of USD 40,000", "EUR")
	if !ok || minor != 4_000_000 || currency != "USD" || assumed {
		t.Fatalf("QuotedBudget = %d %s assumed=%v ok=%v, want 4000000 USD as stated", minor, currency, assumed, ok)
	}
}

func TestABudgetIsNotGuessedWhenItsWordsAreUnclear(t *testing.T) {
	t.Parallel()
	for _, quote := range []string{
		"between 10,000 and 15,000", "10,000 USD or 9,000 EUR", "about fifty thousand", "budget for 2027: 30,000",
	} {
		if minor, currency, _, ok := QuotedBudget(quote, "EUR"); ok {
			t.Errorf("QuotedBudget(%q) = %d %s; want no budget", quote, minor, currency)
		}
	}
	if _, _, _, ok := QuotedBudget("our budget is 10,000", ""); ok {
		t.Error("a budget with no currency was read with no fallback to read it in")
	}
}
