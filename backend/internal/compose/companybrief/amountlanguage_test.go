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

const fleetDealID = "44444444-4444-4444-8444-444444444444"

func twoDealAccount() Input {
	return Input{
		ID: briefCompanyID, Name: "Acme",
		OpenDeals: []DealIn{
			{ID: fleetDealID, Name: "Fleet retrofit", AmountMinor: 5_000_000, Currency: "EUR"},
			{ID: "55555555-5555-4555-8555-555555555555", Name: "Depot", AmountMinor: 7_000_000, Currency: "EUR"},
		},
		WonLifetime: 3_000_000, WonCurrency: "EUR",
	}
}

func TestTheTotalsMargincePutsOnTheCardAreGroundedAndNoOtherSumIs(t *testing.T) {
	in := twoDealAccount()
	known := knownRecords(in.ID, in)
	company := []Evidence{{EntityType: citeCompany, EntityID: in.ID}}
	deal := []Evidence{{EntityType: citeDeal, EntityID: fleetDealID}}
	cases := []struct {
		name     string
		sentence Sentence
		kept     bool
	}{
		{"the open total", Sentence{Text: "Open pipeline is 120,000.00 EUR.", Evidence: company}, true},
		{"the won total", Sentence{Text: "Won to date is 30,000.00 EUR.", Evidence: company}, true},
		{"a sum that is no row and no total", Sentence{Text: "Open pipeline is 95,000.00 EUR.", Evidence: company}, false},
		{"the largest row beside a sum", Sentence{Text: "Fleet retrofit at 50,000.00 EUR makes 120,000.00 EUR.", Evidence: deal}, false},
		{"one row's own amount", Sentence{Text: "Fleet retrofit is 50,000.00 EUR.", Evidence: deal}, true},
	}
	for _, c := range cases {
		if got := claims.UngroundedExactMoney(c.sentence, known) == ""; got != c.kept {
			t.Errorf("%s: kept = %v, want %v", c.name, got, c.kept)
		}
	}
}

func TestNoOpenTotalIsGroundedWhenTheDealsMixCurrencies(t *testing.T) {
	in := twoDealAccount()
	in.OpenDeals[1].Currency = "USD"
	sentence := Sentence{Text: "Open pipeline is 120,000.00 EUR.", Evidence: []Evidence{{EntityType: citeCompany, EntityID: in.ID}}}
	if claims.UngroundedExactMoney(sentence, knownRecords(in.ID, in)) == "" {
		t.Error("a total the floor would never state was kept")
	}
}

func TestBothParsersKeepTheOpenTotalAndRefuseAnyOtherSum(t *testing.T) {
	in := twoDealAccount().WordedIn("en")
	sentence := func(text string) string {
		return `{"text":"` + text + `","nature":"fact","evidence":[{"entity_type":"company","entity_id":"` + in.ID + `"}]}`
	}
	cases := []struct {
		text string
		kept int
	}{{"Open pipeline is 120,000.00 EUR.", 1}, {"Open pipeline is 95,000.00 EUR.", 0}}
	for _, c := range cases {
		flat, err := parseCitedBrief(`{"sentences":[`+sentence(c.text)+`]}`, in)
		if err != nil || len(flat) != c.kept {
			t.Errorf("flat parser on %q kept %d (%v), want %d", c.text, len(flat), err, c.kept)
		}
		sections, err := parseCitedSections(`{"sections":[{"kind":"health","sentences":[`+sentence(c.text)+`]}]}`, in.ID, in)
		if err != nil || len(sections) != c.kept {
			t.Errorf("sectioned parser on %q kept %d sections (%v), want %d", c.text, len(sections), err, c.kept)
		}
	}
}
