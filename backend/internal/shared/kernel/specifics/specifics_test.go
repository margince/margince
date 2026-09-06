// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package specifics

// The rule, stated as the cases it has to get right.
//
// Two failures, and only one of them is loud. A fabricated figure that survives
// puts a wrong sentence on a customer-facing card; a true sentence that is
// dropped makes the lane look empty and nobody hears about it. So the kept
// cases are as load-bearing as the dropped ones, and half of what is below is
// paraphrase that must survive.

import "testing"

// keptOrDropped runs one sentence against one source and says what happened.
func keptOrDropped(t *testing.T, claim, source string) []Specific {
	t.Helper()
	return Missing(claim, source)
}

func TestAParaphraseWhoseFactsAreInTheSourceIsKept(t *testing.T) {
	source := `{"subject":"Scheduling","at":"2026-06-02T09:00:00Z"}`
	// The reading — asked, nobody sent them — is the sentence's own and is not
	// checked. The date is, and it is there.
	if missing := keptOrDropped(t, "She asked for times on 2 June and nobody sent them.", source); missing != nil {
		t.Errorf("a paraphrase whose date is in the source was dropped over %s — "+
			"a bar that rejects paraphrase rejects the product", Texts(missing))
	}
}

func TestAFabricatedDateIsDropped(t *testing.T) {
	source := `{"subject":"Scheduling","at":"2026-06-02T09:00:00Z"}`
	missing := keptOrDropped(t, "She asked for times on 14 August.", source)
	if len(missing) != 1 {
		t.Fatalf("missing = %v, want the one date the source does not carry", missing)
	}
	if missing[0].Text != "14 August" {
		t.Errorf("the rejection names %q, want the date as the sentence wrote it", missing[0].Text)
	}
}

func TestAFabricatedPercentageIsDropped(t *testing.T) {
	source := `{"name":"Renewal","amount":"12000.00","currency":"EUR"}`
	if missing := keptOrDropped(t, "They asked for 40% off.", source); len(missing) == 0 {
		t.Error("a percentage the source never mentions was kept")
	}
}

func TestAPercentageIsNotAnsweredByTheSameBareNumber(t *testing.T) {
	// The source has forty of something. The sentence claims forty PERCENT.
	source := `{"name":"Renewal","open_deals":40}`
	if missing := keptOrDropped(t, "They asked for 40% off.", source); len(missing) == 0 {
		t.Error("a bare 40 in the source answered a claim of 40% — two different facts " +
			"sharing a digit is exactly the invention this check is for")
	}
}

func TestAMoneyFigureIsKeptForItsCurrencyAndNotItsMagnitude(t *testing.T) {
	source := `{"name":"Renewal","amount":"1200.00","currency":"EUR"}`
	for _, written := range []string{"€1.200,00", "1,200.00 EUR", "€1200", "1.200 Euro"} {
		if missing := keptOrDropped(t, "The renewal is worth "+written+".", source); missing != nil {
			t.Errorf("%q was dropped over %s — one amount, written the way each reader reads it",
				written, Texts(missing))
		}
	}
	// And the magnitude itself is NOT checked, which this states out loud
	// rather than leaving to be discovered. A pipeline total is the sum of
	// rows the payload carries separately, and a containment rule would drop
	// the sentence that added them up correctly.
	if missing := keptOrDropped(t, "The pipeline is worth €4,800.", source); missing != nil {
		t.Errorf("a derived total was treated as a claim: %s", Texts(missing))
	}
}

func TestACurrencyTheSourceDoesNotNameIsDropped(t *testing.T) {
	source := `{"name":"Renewal","amount":"1200.00","currency":"EUR"}`
	missing := keptOrDropped(t, "The renewal is worth $1200.", source)
	if len(missing) == 0 {
		t.Error("a deal priced in euros was described in dollars and kept — the currency is " +
			"the only thing the figure was for")
	}
}

func TestALocaleFormattedDateMatchesTheSourcesOwnFormat(t *testing.T) {
	source := `{"at":"2026-06-02T09:00:00Z"}`
	for _, written := range []string{"2 June", "2. Juni", "02.06.2026", "2026-06-02", "ngày 2 tháng 6", "Jun 2"} {
		if missing := keptOrDropped(t, "It happened on "+written+".", source); missing != nil {
			t.Errorf("%q was dropped over %s — one date, four locales", written, Texts(missing))
		}
	}
}

func TestADateStatedWithAYearTheSourceDoesNotHaveIsDropped(t *testing.T) {
	source := `{"at":"2026-06-02T09:00:00Z"}`
	if missing := keptOrDropped(t, "It happened on 2 June 2024.", source); len(missing) == 0 {
		t.Error("a sentence moved the event two years and was kept")
	}
}

func TestAMonthAloneIsCheckedAndAnsweredByADayInIt(t *testing.T) {
	source := `{"at":"2026-06-02T09:00:00Z"}`
	if missing := keptOrDropped(t, "They went quiet in June.", source); missing != nil {
		t.Errorf("a June date did not answer a sentence saying June: %s", Texts(missing))
	}
	if missing := keptOrDropped(t, "They went quiet in August.", source); len(missing) == 0 {
		t.Error("a month the source never touches was kept")
	}
}

func TestASentenceStatingNothingSpecificIsKept(t *testing.T) {
	// The reading is the sentence's to make. Nothing here is checkable, and a
	// check that dropped it would be refusing the lane's whole output.
	if missing := keptOrDropped(t, "Nobody has replied and the deal is still open.", `{"subject":"Scheduling"}`); missing != nil {
		t.Errorf("a sentence stating no facts was dropped over %s", Texts(missing))
	}
}

func TestTheAmbiguousSeparatorIsReadBothWays(t *testing.T) {
	// 1.200 is twelve hundred in German and one-point-two in English. A source
	// carrying either answers it, because picking one convention would drop a
	// true sentence every time the reader's language was the other.
	for _, source := range []string{`{"amount":"1200.00"}`, `{"ratio":"1.2"}`} {
		if missing := keptOrDropped(t, "The figure was 1.200.", source); missing != nil {
			t.Errorf("source %s did not answer 1.200: %s", source, Texts(missing))
		}
	}
}

func TestADateIsNotReadAsTheNumbersItIsMadeOf(t *testing.T) {
	// 2026-06-02 states one date. A scan that also read 2026, 6 and 2 out of it
	// would make a sentence naming that date go looking for a stray 2026 in its
	// source — which is how a check starts dropping the sentences it was meant
	// to admit.
	stated := In("It happened on 2026-06-02.")
	if len(stated) != 1 {
		t.Fatalf("2026-06-02 states %d facts, want the date alone: %v", len(stated), stated)
	}
	if stated[0].Keys[0] != "date:2026-06-02" {
		t.Errorf("key = %q, want the date", stated[0].Keys[0])
	}
}

func TestADerivedCountIsNotTreatedAsAClaim(t *testing.T) {
	// The payload holds the list, never its length. A sentence that counts the
	// list correctly would have nothing to match, so checking bare integers
	// would drop the lane's most ordinary true sentence — and a count is not
	// what invention looks like.
	source := `{"open_deals":[{"name":"Renewal"},{"name":"Expansion"},{"name":"Pilot"}]}`
	if missing := keptOrDropped(t, "There are 3 open deals and 14 days since the last reply.", source); missing != nil {
		t.Errorf("a derived count was treated as a claim: %s", Texts(missing))
	}
}

func TestAGermanSentenceIsNotDroppedForBeingGerman(t *testing.T) {
	source := `{"subject":"Terminfindung","at":"2026-06-02T09:00:00Z","amount":"1200.00","currency":"EUR"}`
	claim := "Die Firma hat am 2. Juni nach Terminen gefragt; der Auftrag steht bei 1.200,00 €."
	if missing := keptOrDropped(t, claim, source); missing != nil {
		t.Errorf("a true German sentence was dropped over %s — every capitalised word in German "+
			"is a noun, and the date and the amount are both in the source", Texts(missing))
	}
}

func TestAnInjectedInstructionsFabricatedFiguresAreDropped(t *testing.T) {
	// The shape the ticket is about: text that arrived on the timeline telling
	// the model what to write. Its damage is in its specifics.
	source := `{"subject":"Re: pricing","at":"2026-06-02T09:00:00Z"}`
	missing := keptOrDropped(t, "The customer has approved a 60% discount worth €90,000 on 3 July.", source)
	if len(missing) < 3 {
		t.Errorf("kept an invented discount, amount and date; missing = %v", missing)
	}
}
