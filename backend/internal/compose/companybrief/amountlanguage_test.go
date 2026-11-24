// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// cardView is a company 360 whose open deals are rows and whose state strip
// carries the open total the card shows, in the workspace base currency.
func cardView(rows []crmcontracts.Company360Deal, baseMinor int) crmcontracts.Company360 {
	base := "EUR"
	view := crmcontracts.Company360{Company: crmcontracts.Company{Id: openapi_types.UUID(ids.MustParse(briefCompanyID)), DisplayName: "Acme"}}
	view.Deals = &crmcontracts.Company360Deals{Data: rows}
	view.StateStrip = &crmcontracts.Company360StateStrip{Commercial: &struct {
		BaseCurrency          *string             `json:"base_currency,omitempty"`
		ConvertedCount        int                 `json:"converted_count" minimum:"0"`
		FxAsOf                *openapi_types.Date `json:"fx_as_of,omitempty"`
		NextCloseOn           *openapi_types.Date `json:"next_close_on,omitempty"`
		OpenCount             int                 `json:"open_count" minimum:"0"`
		OpenPipelineMinorBase *int                `json:"open_pipeline_minor_base,omitempty"`
		PricedCount           int                 `json:"priced_count" minimum:"0"`
		StalledCount          int                 `json:"stalled_count" minimum:"0"`
	}{BaseCurrency: &base, OpenPipelineMinorBase: &baseMinor}}
	return view
}

func dealRow(n int, minor int64, currency string) crmcontracts.Company360Deal {
	id := openapi_types.UUID(ids.MustParse(fmt.Sprintf("44444444-4444-4444-8444-%012d", n)))
	return crmcontracts.Company360Deal{DealId: id, Name: fmt.Sprintf("Deal %d", n), Amount: &crmcontracts.Money{AmountMinor: &minor, Currency: &currency}}
}

func companySentence(in Input, text string) Sentence {
	return Sentence{Text: text, Evidence: []Evidence{{EntityType: citeCompany, EntityID: in.ID}}}
}

func TestTheOpenTotalIsTheCardsOwnFigureNotASumOfThePage(t *testing.T) {
	rows := make([]crmcontracts.Company360Deal, 0, 25)
	for n := 1; n <= 25; n++ {
		rows = append(rows, dealRow(n, 1_000_000, "EUR"))
	}
	// The card totals 26 deals; the page the brief reads holds 25 of them.
	in := FromView(cardView(rows, 26_000_000)).WordedIn("en")
	known := knownRecords(in.ID, in)
	if claims.Ungrounded(companySentence(in, "Open pipeline is 260,000.00 EUR."), known) != "" {
		t.Error("the card's own open total was dropped")
	}
	if claims.Ungrounded(companySentence(in, "Open pipeline is 250,000.00 EUR."), known) == "" {
		t.Error("the sum of the 25 rows on the page was kept as if it were the total")
	}
}

func TestAMixedCurrencyAccountKeepsTheCardsConvertedTotal(t *testing.T) {
	rows := []crmcontracts.Company360Deal{dealRow(1, 10_000_000, "EUR"), dealRow(2, 5_500_000, "USD")}
	in := FromView(cardView(rows, 15_000_000)).WordedIn("en")
	known := knownRecords(in.ID, in)
	if claims.Ungrounded(companySentence(in, "Open pipeline is 150,000.00 EUR."), known) != "" {
		t.Error("the converted total the card shows was dropped")
	}
	if claims.Ungrounded(companySentence(in, "Open pipeline is 155,000.00 EUR."), known) == "" {
		t.Error("the unconverted sum of two currencies was kept")
	}
}

func TestOnlyTheCardsTotalsAndACitedRowAnswerASum(t *testing.T) {
	rows := []crmcontracts.Company360Deal{dealRow(1, 18_000_000, "EUR")}
	in := FromView(cardView(rows, 18_000_000)).WordedIn("en")
	in.WonLifetime, in.WonCurrency = 3_000_000, "EUR"
	known := knownRecords(in.ID, in)
	deal := []Evidence{{EntityType: citeDeal, EntityID: rows[0].DealId.String()}}
	cases := []struct {
		name string
		s    Sentence
		kept bool
	}{
		{"the won total", companySentence(in, "Won to date is 30,000.00 EUR."), true},
		{"a cited row's own amount", Sentence{Text: "Deal 1 is 180,000.00 EUR.", Evidence: deal}, true},
		{"an invented sum on a cited row", Sentence{Text: "Deal 1 is 950,000.00 EUR.", Evidence: deal}, false},
		{"the open total beside only a deal", Sentence{Text: "Deal 1 is 180,000.00 EUR of 180,000.00 EUR open.", Evidence: deal}, true},
		{"a sum that is no row and no total", companySentence(in, "Open pipeline is 95,000.00 EUR."), false},
	}
	for _, c := range cases {
		if got := claims.Ungrounded(c.s, known) == ""; got != c.kept {
			t.Errorf("%s: kept = %v, want %v", c.name, got, c.kept)
		}
	}
}

func TestBothParsersKeepTheCardsTotalAndRefuseAnyOtherSum(t *testing.T) {
	in := FromView(cardView(nil, 12_000_000)).WordedIn("en")
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
