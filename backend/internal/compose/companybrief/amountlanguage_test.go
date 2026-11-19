// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/claims"
)

func germanAccount() Input {
	return Input{
		ID: "c-1", Name: "Acme",
		OpenDeals:   []DealIn{{ID: "d-1", Name: "Fleet retrofit", Stage: "Qualified", AmountMinor: 1_234_500, Currency: "EUR"}},
		WonLifetime: 1_200_000, WonCurrency: "EUR",
	}.WordedIn("de")
}

func TestAGermanBriefReadsAmountsInGermanDigitMarks(t *testing.T) {
	in := germanAccount()
	encoded, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("encoding the input: %v", err)
	}
	for _, want := range []string{`"amount":"12.345,00"`, `"won_lifetime":"12.000,00"`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("the prompt carries %s, want %s", encoded, want)
		}
	}
	floor := claims.Prose(Deterministic(in.ID, in, "de"))
	for _, want := range []string{"12.345,00 EUR", "12.000,00 EUR"} {
		if !strings.Contains(floor, want) {
			t.Errorf("the fallback brief reads %q, want %q in it", floor, want)
		}
	}
	if line := openDealLine(in.OpenDeals[0], companyPhrasesFor("de")); !strings.Contains(line, "12.345,00 EUR") {
		t.Errorf("the open-deal answer reads %q, want 12.345,00 EUR", line)
	}
}

func TestWordingTheInputLeavesTheCallersDealsAsTheyWere(t *testing.T) {
	plain := Input{OpenDeals: []DealIn{{ID: "d-1", AmountMinor: 1_234_500, Currency: "EUR"}}}
	if worded := plain.WordedIn("de"); worded.OpenDeals[0].lang != "de" {
		t.Fatal("the worded copy did not take the language")
	}
	encoded, err := json.Marshal(plain.OpenDeals[0])
	if err != nil {
		t.Fatalf("encoding the deal: %v", err)
	}
	if !strings.Contains(string(encoded), `"amount":"12,345.00"`) {
		t.Errorf("the caller's deal now encodes as %s; wording a copy changed the original", encoded)
	}
}

func TestAGermanFigureTheModelCopiesIsGroundedInItsDeal(t *testing.T) {
	in := germanAccount()
	sentence := Sentence{
		Text:     "Fleet retrofit steht bei 12.345,00 EUR.",
		Evidence: []Evidence{{EntityType: citeDeal, EntityID: "d-1"}},
	}
	if reason := claims.Ungrounded(sentence, knownRecords(in.ID, in)); reason != "" {
		t.Errorf("a figure copied from the deal was dropped: %s", reason)
	}
}
