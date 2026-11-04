// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import (
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/modules/deals"
)

var today = time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)

// fixedRates converts at the stated rates to the base, EUR.
type fixedRates map[string]deals.FXRate

func (r fixedRates) For(currency string) (deals.FXRate, bool, error) {
	rate, ok := r[currency]
	return rate, ok, nil
}

func rate(units int64, exp int32) deals.FXRate {
	return deals.FXRate{Rate: pgtype.Numeric{Int: big.NewInt(units), Exp: exp, Valid: true}, On: today.AddDate(0, 0, -2)}
}

func guardInput() Input {
	return Input{Today: today, Probability: 40, Activities: []Activity{
		{ID: "mail", Kind: "email", Direction: "inbound", OccurredAt: today.AddDate(0, 0, -3),
			Subject: "Order", Body: "We can sign for 50,000 USD for the first year, start in March."},
	}}
}

func guardState() State {
	amount, currency, pct, stage := int64(4_000_000), "EUR", 40, "stage"
	return State{
		Seen:       deals.VerdictValues{AmountMinor: &amount, Currency: &currency, Probability: &pct, ProbabilitySource: &stage},
		SwitchedOn: map[string]bool{deals.VerdictCloseDateCorrection: true, deals.VerdictAmountCorrection: true, deals.VerdictProbabilityCorrection: true},
	}
}

func dateAction(date string) Action {
	return Action{Kind: KindSetCloseDate, Date: date, Cited: []string{"mail"}, Confidence: 0.9, Reason: "r"}
}

func amountAction(amount int64, currency, quote string) Action {
	return Action{Kind: KindSetAmount, Amount: amount, Currency: currency, QuotedWords: quote,
		Cited: []string{"mail"}, Confidence: 0.9, Reason: "r"}
}

func probabilityAction(pct int) Action {
	return Action{Kind: KindSetProbability, Probability: pct, Cited: []string{"mail"}, Confidence: 0.9, Reason: "r"}
}

func TestEachGuardDropsWhatItGuards(t *testing.T) {
	t.Parallel()
	human := "human"
	acceptedEUR := &Offer{GrossMinor: 4_610_000, Currency: "EUR", Accepted: true}
	cases := []struct {
		name   string
		action Action
		state  func(*State)
		want   string
	}{
		{"a date three months out applies", dateAction("2027-02-01"), nil, OutcomeApply},
		{"below the confidence floor", func() Action { a := dateAction("2027-02-01"); a.Confidence = 0.5; return a }(), nil, DroppedLowConfidence},
		{"a date in the past", dateAction("2026-10-01"), nil, DroppedDateOutOfRange},
		{"a date today", dateAction("2026-11-04"), nil, DroppedDateOutOfRange},
		{"a date past eighteen months", dateAction("2028-06-01"), nil, DroppedDateOutOfRange},
		{"the switch is off", dateAction("2027-02-01"), func(s *State) { s.SwitchedOn[deals.VerdictCloseDateCorrection] = false }, DroppedSwitchOff},
		{"an Undo holds it", dateAction("2027-02-01"), func(s *State) { s.Held = map[string]bool{deals.VerdictCloseDateCorrection: true} }, DroppedUndoHold},
		{"a colleague edited the date after the mail", dateAction("2027-02-01"), func(s *State) {
			s.HumanEditedAt = map[string]time.Time{deals.VerdictCloseDateCorrection: today.AddDate(0, 0, -1)}
		}, DroppedHumanEdit},
		{"a colleague edited the date before the mail", dateAction("2027-02-01"), func(s *State) {
			s.HumanEditedAt = map[string]time.Time{deals.VerdictCloseDateCorrection: today.AddDate(0, 0, -9)}
		}, OutcomeApply},
		{"a probability applies", probabilityAction(70), nil, OutcomeApply},
		{"a probability a colleague set", probabilityAction(70), func(s *State) { s.Seen.ProbabilitySource = &human }, DroppedHumanSet},
		{"the probability it already has", probabilityAction(40), nil, DroppedUnchanged},
		{"an amount whose words are in the mail", amountAction(50_000, "USD", "50,000 USD for the first year"), nil, OutcomeApply},
		{"an amount whose words are not", amountAction(50_000, "USD", "60,000 USD"), nil, DroppedQuoteNotFound},
		{"an amount in a currency with no rate", amountAction(50_000, "GBP", "for the first year"), nil, DroppedNoRate},
		{"an accepted offer fixes the amount", amountAction(50_000, "USD", "50,000 USD"), func(s *State) { s.AcceptedOffer = acceptedEUR }, DroppedOfferFixes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := guardState()
			if tc.state != nil {
				tc.state(&state)
			}
			got, err := Decide(guardInput(), state, Payload{Actions: []Action{tc.action}}, fixedRates{"EUR": rate(1, 0), "USD": rate(922, -3)})
			if err != nil {
				t.Fatal(err)
			}
			if got[0].Outcome != tc.want {
				t.Fatalf("outcome = %q, want %q", got[0].Outcome, tc.want)
			}
		})
	}
}

// 50,000 USD at 0.922 EUR per USD is 46,100 EUR, and the conversion is kept.
func TestAnAmountInAnotherCurrencyIsConvertedThroughTheBase(t *testing.T) {
	t.Parallel()
	got, err := Decide(guardInput(), guardState(),
		Payload{Actions: []Action{amountAction(50_000, "USD", "50,000 USD for the first year")}},
		fixedRates{"EUR": rate(1, 0), "USD": rate(922, -3)})
	if err != nil {
		t.Fatal(err)
	}
	change := got[0].Change
	if change == nil || change.AmountMinor != 4_610_000 || change.Currency != "" {
		t.Fatalf("change = %+v, want 46,100.00 EUR in the deal's own currency", change)
	}
	if change.Evidence["quoted_currency"] != "USD" || change.Evidence["rate_from"] != "0.922" {
		t.Fatalf("evidence = %v, want the quote and the rate kept", change.Evidence)
	}
}

// The deal's currency converts the other way: 1 USD at 0.922 EUR, so a deal
// in USD quoted 1,000 EUR is 1,085 USD after rounding to whole units.
func TestAConversionRoundsToWholeUnits(t *testing.T) {
	t.Parallel()
	usd := "USD"
	state := guardState()
	state.Seen.Currency = &usd
	got, err := Decide(guardInput(), state,
		Payload{Actions: []Action{amountAction(1_000, "EUR", "first year")}},
		fixedRates{"EUR": rate(1, 0), "USD": rate(922, -3)})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Change == nil || got[0].Change.AmountMinor != 108_500 {
		t.Fatalf("change = %+v, want 1,085.00 USD", got[0].Change)
	}
}

// An accepted offer lets the amount become exactly that offer's.
func TestAnAcceptedOfferSetsItsOwnAmount(t *testing.T) {
	t.Parallel()
	state := guardState()
	state.AcceptedOffer = &Offer{GrossMinor: 5_000_000, Currency: "EUR", Accepted: true}
	in := guardInput()
	in.Activities[0].Body = "Accepted: 50,000 EUR."
	got, err := Decide(in, state, Payload{Actions: []Action{amountAction(50_000, "EUR", "50,000 EUR")}}, fixedRates{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Change == nil || got[0].Change.AmountMinor != 5_000_000 {
		t.Fatalf("change = %+v, want the offer's gross amount", got[0].Change)
	}
}

func TestAnUndoHoldsUntilSomethingNewArrives(t *testing.T) {
	t.Parallel()
	undone := map[string]time.Time{deals.VerdictAmountCorrection: today}
	if !Holds(undone, today.Add(-time.Hour))[deals.VerdictAmountCorrection] {
		t.Fatal("evidence older than the Undo must hold the correction back")
	}
	if Holds(undone, today.Add(time.Hour))[deals.VerdictAmountCorrection] {
		t.Fatal("evidence after the Undo must lift the hold")
	}
}

func TestTheValidatorRefusesACitationOutsideTheInput(t *testing.T) {
	t.Parallel()
	text := `{"actions":[{"kind":"set_probability","date":"","amount":0,"currency":"","quoted_words":"",` +
		`"probability":60,"cited_activity_ids":["other"],"reason":"r","confidence":0.9}]}`
	if _, err := Parse(text, guardInput()); err == nil {
		t.Fatal("a citation of an activity that was not supplied must be refused")
	}
	if _, err := Parse(`{"actions":[]}`, guardInput()); err != nil {
		t.Fatalf("an empty answer is valid: %v", err)
	}
}
