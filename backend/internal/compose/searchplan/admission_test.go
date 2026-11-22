// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package searchplan

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSearchMoneyFiltersUseTheCurrencyScaleAtEveryQueryLevel(t *testing.T) {
	for _, test := range []struct{ currency, major, minor string }{
		{"EUR", "50000", "5000000"}, {"JPY", "50000", "50000"}, {"KWD", "50.125", "50125"},
	} {
		t.Run(test.currency, func(t *testing.T) {
			predicate := `[{"field":"currency","op":"eq","value":"` + test.currency + `"},{"field":"amount_minor","op":"gt","value":` + test.major + `}]`
			for _, query := range []string{`{"plan":{"where":` + predicate + `}}`, `{"plan":{"traverse":{"where":` + predicate + `}}}`, `{"filters":` + predicate + `}`} {
				got, err := searchMoneyOperands(json.RawMessage(query), "EUR")
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(got), `"value":`+test.minor+`}`) {
					t.Fatalf("money not normalized: %s", got)
				}
			}
		})
	}
	for _, query := range []string{
		`{"where":[{"field":"amount_minor","op":"gt","value":50000}]}`,
		`{"where":[{"field":"currency","op":"eq","value":"JPY"},{"field":"amount_minor","op":"gt","value":0.1}]}`,
		`{"where":[{"field":"amount_minor_base","op":"gt","value":50000,"ignored":true}]}`,
	} {
		if _, err := searchMoneyOperands(json.RawMessage(query), "EUR"); err == nil {
			t.Fatalf("accepted ambiguous or inexact filter: %s", query)
		}
	}
	got, err := searchMoneyOperands(json.RawMessage(`{"filters":[{"field":"amount_minor_base","op":"between","values":[1,2]}]}`), "EUR")
	if err != nil || !strings.Contains(string(got), `"values":[100,200]`) {
		t.Fatalf("base currency: %s, %v", got, err)
	}
}

func TestSearchWonDealYearsIncludeBothWholeYearsInTheInstallationZone(t *testing.T) {
	const vocabulary = `{"context":{"timezone":"Europe/Berlin"}}`
	const question = "How many deals did I win from 2024 to 2026?"
	const arguments = `{"scope":{"kind":"workspace"},"period":"custom","interval":{"start_at":"2024-01-01T00:00:00+01:00","end_at":"2027-01-01T00:00:00+01:00"},"metrics":["bookings_won"]}`
	plan := Plan{Engine: "reporting", Count: true, Arguments: arguments}
	if _, err := Admit(plan, question, vocabulary); err != nil {
		t.Fatal(err)
	}
	plan.Arguments = strings.ReplaceAll(arguments, "2027-01-01", "2026-01-01")
	if _, err := Admit(plan, question, vocabulary); err == nil {
		t.Fatal("a truncated last year was accepted")
	}
	plan.Arguments = strings.ReplaceAll(arguments, "+01:00", "Z")
	if _, err := Admit(plan, question, vocabulary); err == nil {
		t.Fatal("UTC midnight was accepted for a Berlin interval")
	}
	plan = Plan{Engine: "records", Arguments: "{}"}
	if _, err := Admit(plan, question, vocabulary); err == nil {
		t.Fatal("a bounded list was accepted as a won-deal count")
	}
	plan.UnsupportedParts = []string{"unavailable condition"}
	got, err := Admit(plan, question, vocabulary)
	if err != nil || got.Engine != "unsupported" {
		t.Fatalf("unsupported phrases must prevent a partial answer: %+v, %v", got, err)
	}
}

func TestAWonCountInEitherWordOrderUsesSalesWon(t *testing.T) {
	const vocabulary = `{"context":{"timezone":"Asia/Ho_Chi_Minh","base_currency":"VND"}}`
	for _, question := range []string{
		"tháng này tôi thắng bao nhiêu deal",
		"thang nay toi thang bao nhieu deal",
		"bao nhiêu deal tôi thắng tháng này",
		"tháng này tôi thắng mấy deal",
		"thang nay toi thang may deal bao nhieu",
		"How many deals did we win?",
	} {
		if _, err := Admit(Plan{Engine: "records", Arguments: `{"plan":{"version":"v1","target":"deal"}}`}, question, vocabulary); err == nil {
			t.Errorf("%q: a record list was accepted as a won-deal count", question)
		}
		won := Plan{Engine: "reporting", Count: true, Arguments: `{"scope":{"kind":"workspace"},"period":"month","metrics":["bookings_won"]}`}
		if _, err := Admit(won, question, vocabulary); err != nil {
			t.Errorf("%q: the Sales won count was refused: %v", question, err)
		}
	}
}

func TestAListThatOnlyResemblesAWonCountStaysAList(t *testing.T) {
	const vocabulary = `{"context":{"timezone":"UTC","base_currency":"EUR"}}`
	for _, question := range []string{
		"list the deals won with a discount",
		"show deals won by key accounts",
		"which deals did we win in each country",
		"how many deals are in the Winterhalter window",
		"danh sách deal thắng của khách hàng mấy tháng gần đây",
		"bao nhieu deal mo trong thang nay",
		"bao nhieu deal mo thang truoc",
	} {
		if _, err := Admit(Plan{Engine: "records", Arguments: `{"plan":{"version":"v1","target":"deal"}}`}, question, vocabulary); err != nil {
			t.Errorf("%q: a record list was refused as a won count: %v", question, err)
		}
	}
}

// The vocabulary a planner reads says the unit admission scales from: a field
// it scales reads major units in the vocabulary's words, and a 50k threshold on
// amount_minor crosses into the stored minor units once, never twice.
func TestTheVocabularySaysTheUnitAdmissionScalesFrom(t *testing.T) {
	encoded, err := json.Marshal(Vocabulary{})
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	var wire struct {
		MoneyOperands string `json:"money_operands"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil || !strings.Contains(wire.MoneyOperands, "MAJOR") {
		t.Fatalf("money_operands = %q (%v), want the major-unit rule", wire.MoneyOperands, err)
	}
	for _, field := range []string{"amount_minor", "amount_minor_base"} {
		if !scaledMoneyField(field) || !strings.Contains(wire.MoneyOperands, strings.TrimPrefix(field, "amount")) {
			t.Errorf("%s: scaled %v, named by the rule %v", field, scaledMoneyField(field), strings.Contains(wire.MoneyOperands, strings.TrimPrefix(field, "amount")))
		}
	}
	got, err := searchMoneyOperands(json.RawMessage(`{"where":[{"field":"currency","op":"eq","value":"EUR"},{"field":"amount_minor","op":"gt","value":50000}]}`), "EUR")
	if err != nil || !strings.Contains(string(got), `"value":5000000`) {
		t.Errorf("a 50000 EUR threshold admitted as %s (%v), want 5000000 minor units", got, err)
	}
}
