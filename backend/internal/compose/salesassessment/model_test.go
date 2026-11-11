// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

import (
	"slices"
	"strings"
	"testing"
)

func assessmentInput() Input {
	return Input{
		Frame: FrameIn{Scope: "Whole company", Period: "this_quarter", Currency: "EUR", Today: "2026-11-03"},
		Cells: []CellIn{
			{ID: "metric:bookings_won", Label: "bookings_won", Value: "216000 EUR", Unit: "EUR", Coverage: "ok"},
			{ID: "metric:open_pipeline", Label: "open_pipeline", Value: "370000 EUR", Unit: "EUR", Coverage: "ok"},
		},
		Candidates: []CandidateIn{{ID: "deal-1", Name: "Linden press", Amount: "250000 EUR", OwnerID: "user-1"}},
	}
}

func TestParseKeepsASentenceWhoseNumbersTheCitedCellsCarry(t *testing.T) {
	written, err := Parse(`{"sentences":[{"text":"Sales won stand at €216,000 against €370k open.","kind":"observation","cells":["metric:bookings_won","metric:open_pipeline"]}],"next_decision":null,"confidence":"high"}`, assessmentInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(written.Sentences) != 1 || written.Sentences[0].Kind != "observation" || written.Confidence != "high" {
		t.Fatalf("kept %+v", written)
	}
}

func TestParseDropsASentenceQuotingANumberNoCitedCellCarries(t *testing.T) {
	written, err := Parse(`{"sentences":[{"text":"Sales won stand at 216000 EUR.","kind":"observation","cells":["metric:bookings_won"]},{"text":"That is 58 percent of the target.","kind":"hypothesis","cells":["metric:bookings_won"]},{"text":"Open pipeline is 216000 EUR.","kind":"claim","cells":["metric:open_pipeline"]}],"next_decision":null,"confidence":"medium"}`, assessmentInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(written.Sentences) != 1 || !strings.HasPrefix(written.Sentences[0].Text, "Sales won") {
		t.Fatalf("kept %+v, want the one sentence whose figure its cell carries", written.Sentences)
	}
	guessed, err := Parse(`{"sentences":[{"text":"Sales won stand at 216000 EUR.","kind":"observation","cells":["metric:bookings_won"]},{"text":"About 21 deals closed.","kind":"observation","cells":["metric:bookings_won"]},{"text":"Two deals carry 2 EUR.","kind":"observation","cells":["metric:bookings_won"]}],"next_decision":null}`, assessmentInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(guessed.Sentences) != 1 {
		t.Fatalf("kept %+v, want a two-digit opening of a figure and a one-digit guess dropped", guessed.Sentences)
	}
}

func TestParseDropsADecisionQuotingANumberNoCellCarries(t *testing.T) {
	written, err := parseCited(`{"sentences":[{"text":"Sales won stand at 216000 EUR.","kind":"observation","cells":["metric:bookings_won"]}],"next_decision":{"deal_id":"deal-1","text":"Offer a 15 percent discount."},"confidence":"low"}`, assessmentInput())
	if err != nil {
		t.Fatal(err)
	}
	if written.Next != nil {
		t.Fatalf("a decision with an invented figure was kept: %+v", written.Next)
	}
}

func TestParseRefusesAReplyWithNoGroundedSentence(t *testing.T) {
	if _, err := Parse(`{"sentences":[{"text":"All good.","kind":"observation","cells":["metric:unknown"]}],"next_decision":null,"confidence":"low"}`, assessmentInput()); err == nil {
		t.Fatal("a reply citing nothing known was kept")
	}
	if _, err := Parse(`{"sentences":[{"text":"The deal deal-1 is late.","kind":"observation","cells":["metric:bookings_won"]}],"next_decision":null}`, assessmentInput()); err == nil {
		t.Fatal("a reply spelling an id in reader text was kept")
	}
	if _, err := Parse(`{"sentences":[{"text":"See 0198F3A1-7C42-7E0B-9D51-2A6F4B8C1E07 for the figure.","kind":"observation","cells":["metric:bookings_won"]}],"next_decision":null}`, assessmentInput()); err == nil {
		t.Fatal("a reply spelling a uuid the input never carried was kept")
	}
}

func TestParseKeepsOnlyADecisionOnACandidateWithItsOwner(t *testing.T) {
	kept := `{"sentences":[{"text":"Sales won stand at 216000 EUR.","kind":"observation","cells":["metric:bookings_won"]}],"next_decision":{"deal_id":"deal-1","text":"Ask for the budget owner."},"confidence":"low"}`
	written, err := parseCited(kept, assessmentInput())
	if err != nil {
		t.Fatal(err)
	}
	if written.Next == nil || written.Next.OwnerID != "user-1" {
		t.Fatalf("the decision was %+v, want the candidate's owner filled in", written.Next)
	}
	for name, reply := range map[string]string{
		"another deal":  `{"sentences":[{"text":"Sales won stand at 216000 EUR.","kind":"observation","cells":["metric:bookings_won"]}],"next_decision":{"deal_id":"deal-9","text":"Call them."}}`,
		"another owner": `{"sentences":[{"text":"Sales won stand at 216000 EUR.","kind":"observation","cells":["metric:bookings_won"]}],"next_decision":{"deal_id":"deal-1","owner_id":"user-2","text":"Call them."}}`,
	} {
		written, err := parseCited(reply, assessmentInput())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if written.Next != nil {
			t.Fatalf("%s: a decision outside the candidates was kept: %+v", name, written.Next)
		}
	}
}

func TestQuantitiesRequireEquivalentValuesRatherThanDigitPrefixes(t *testing.T) {
	cells := map[string]CellIn{"money": {Value: "216000 EUR", Unit: "EUR"}}
	for text, want := range map[string]bool{"€216k": true, "216,000 EUR": true, "216.000 EUR": true, "€2.16m": false, "216 deals": false, "216000 percent": false} {
		if got := numbersCarried(text, []string{"money"}, cells); got != want {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
}

func TestParagraphsPreserveTheReadingAndBoundTheLegacyProjection(t *testing.T) {
	in := assessmentInput()
	in.Actions = []ActionIn{{ID: "review-open", Cell: "metric:open_pipeline"}}
	sentence := `{"text":"The open pipeline of 370000 EUR is the starting point for the review.","kind":"observation","cells":["metric:open_pipeline"]}`
	reply := `{"paragraphs":[[` + strings.Join([]string{sentence, sentence, sentence, sentence}, ",") + `],[` + strings.Join([]string{sentence, sentence, sentence, sentence}, ",") + `]],"action_ids":["review-open"]}`
	got, err := Parse(reply, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Paragraphs) != 2 || len(got.Paragraphs[1]) != 4 || len(got.Sentences) != 6 || len(got.ActionIDs) != 1 {
		t.Fatalf("paragraphs and legacy projection: %+v", got)
	}
}

func TestParagraphsCannotKeepAnUnsupportedClaimOrInventAnAction(t *testing.T) {
	in := assessmentInput()
	for _, reply := range []string{
		`{"paragraphs":[[{"text":"The pipeline is 42 EUR.","cells":["metric:open_pipeline"]}]]}`,
		`{"paragraphs":[[{"text":"The pipeline is 370000 EUR.","cells":["metric:open_pipeline"]}]],"action_ids":["send-email"]}`,
	} {
		if _, err := Parse(reply, in); err == nil {
			t.Fatalf("unsupported interpretation was kept: %s", reply)
		}
	}
	in.Cells[1].Coverage = "failed"
	if _, err := Parse(`{"paragraphs":[[{"text":"The pipeline is 370000 EUR.","cells":["metric:open_pipeline"]}]]}`, in); err == nil {
		t.Fatal("failed reading supported a claim")
	}
}

func TestAParagraphWrittenAsOneSentenceObjectReadsAsAOneSentenceParagraph(t *testing.T) {
	sentence := `{"text":"The open pipeline of 370000 EUR is the starting point.","kind":"observation","cells":["metric:open_pipeline"]}`
	for name, reply := range map[string]string{
		"a list of sentences": `{"paragraphs":[[` + sentence + `],[` + sentence + `]]}`,
		"a bare sentence":     `{"paragraphs":[` + sentence + `,` + sentence + `]}`,
	} {
		got, err := Parse(reply, assessmentInput())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(got.Paragraphs) != 2 || len(got.Paragraphs[0]) != 1 || len(got.Sentences) != 2 {
			t.Fatalf("%s: read %+v, want two one-sentence paragraphs", name, got)
		}
	}
}

func TestABareSentenceParagraphStillPassesTheSentenceChecks(t *testing.T) {
	for reply, refusal := range map[string]string{
		`{"paragraphs":[{"text":"The pipeline is 42 EUR.","cells":["metric:open_pipeline"]}]}`:            "unsupported sentence",
		`{"paragraphs":[{"text":"You still owe r1.","cells":["metric:open_pipeline"],"records":["r1"]}]}`: "spells an id",
	} {
		if _, err := Parse(reply, decisionInput()); err == nil || !strings.Contains(err.Error(), refusal) {
			t.Fatalf("Parse(%s) = %v, want the %q refusal", reply, err, refusal)
		}
	}
}

func decisionInput() Input {
	return Input{
		Frame: FrameIn{Scope: "Whole company", Period: "this_quarter", Currency: "EUR", Today: "2026-11-03"},
		Cells: []CellIn{
			{ID: "metric:bookings_won", Label: "bookings_won", Value: "0 EUR", Unit: "EUR", Coverage: "ok"},
			{ID: "metric:open_pipeline", Label: "open_pipeline", Value: "37000 EUR", Unit: "EUR", Coverage: "ok"},
		},
		Candidates: []CandidateIn{{ID: "deal-globex", Name: "Globex renewal", Amount: "25000 EUR", Display: "25,000 EUR", OwnerID: "user-1"}},
		Decisions:  []DecisionIn{{ID: "decision-1", Text: "Call the buyer about the budget owner", Deal: "deal-globex", OwnerID: "user-1", ReviewOn: "2026-11-17", Status: "open"}},
	}
}

func TestParagraphsKeepSentencesThatReadADecisionAndADealAmount(t *testing.T) {
	reply := `{"paragraphs":[` +
		`[{"text":"Nothing is won yet against {{metric:open_pipeline}} open.","kind":"observation","cells":["metric:bookings_won","metric:open_pipeline"]}],` +
		`[{"text":"An open decision to call the buyer about the budget owner, due 2026-11-17, still covers the Globex renewal.","kind":"claim","cells":["metric:open_pipeline"],"records":["decision-1"]}],` +
		`[{"text":"The Globex renewal at 25000 EUR carries most of the open pipeline.","kind":"hypothesis","cells":["metric:open_pipeline"],"records":["deal-globex"]},` +
		`{"text":"The Globex renewal's {{deal-globex}} is the figure to protect.","kind":"hypothesis","cells":["metric:open_pipeline"],"records":["deal-globex"]}]` +
		`],"next_decision":null}`
	written, err := parseCited(reply, decisionInput())
	if err != nil {
		t.Fatalf("a reply acknowledging the open decision and quoting the deal's amount was refused: %v", err)
	}
	if len(written.Paragraphs) != 3 {
		t.Fatalf("kept %+v, want all three paragraphs", written.Paragraphs)
	}
	for _, sentence := range written.Paragraphs[2] {
		if !strings.Contains(sentence.Text, "25,000 EUR") || !slices.Equal(sentence.Cells, []string{"metric:open_pipeline"}) {
			t.Fatalf("kept %+v, want the deal's display amount and only the cell among its cells", sentence)
		}
	}
}

func TestParseDropsARecordSentenceWhoseNumberTheCitedRecordDoesNotCarry(t *testing.T) {
	won := `{"text":"Nothing is won yet at 0 EUR.","kind":"observation","cells":["metric:bookings_won"]}`
	for name, line := range map[string]string{
		"an amount the decision never held":     `{"text":"The open decision on the Globex renewal is worth 30000 EUR.","cells":["metric:open_pipeline"],"records":["decision-1"]}`,
		"a delay the decision never held":       `{"text":"The open decision is 5 days late.","cells":["metric:open_pipeline"],"records":["decision-1"]}`,
		"the deal's amount without citing it":   `{"text":"The Globex renewal is worth 25000 EUR.","cells":["metric:open_pipeline"]}`,
		"a decision cited with no cell to read": `{"text":"A call to the buyer about the budget owner is still owed.","cells":[],"records":["decision-1"]}`,
	} {
		written, err := parseCited(`{"sentences":[`+won+`,`+line+`],"next_decision":null}`, decisionInput())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(written.Sentences) != 1 {
			t.Fatalf("%s: kept %+v, want only the sentence its cell carries", name, written.Sentences)
		}
	}
}

func TestParseKeepsADecisionsReviewDayWrittenAsADate(t *testing.T) {
	won := `{"text":"Nothing is won yet at 0 EUR.","kind":"observation","cells":["metric:bookings_won"]}`
	for _, text := range []string{"The call to the buyer is due 17 November.", "The call to the buyer is due November 17."} {
		line := `{"text":"` + text + `","kind":"claim","cells":["metric:open_pipeline"],"records":["decision-1"]}`
		written, err := parseCited(`{"sentences":[`+won+`,`+line+`],"next_decision":null}`, decisionInput())
		if err != nil || len(written.Sentences) != 2 {
			t.Errorf("%q: kept %+v (%v), want the review day read as the decision's own", text, written.Sentences, err)
		}
	}
}

func TestParseRefusesARecordIDSpelledInText(t *testing.T) {
	for _, text := range []string{"The deal deal-globex is still open.", "The decision decision-1 is still owed."} {
		reply := `{"sentences":[{"text":"` + text + `","cells":["metric:open_pipeline"],"records":["deal-globex","decision-1"]}],"next_decision":null}`
		if _, err := Parse(reply, decisionInput()); err == nil {
			t.Fatalf("a reply spelling a record id was kept: %s", text)
		}
	}
}

func TestParseRefusesAShownRefOnlyAsAWordOfItsOwn(t *testing.T) {
	won := `{"text":"Nothing is won yet at 0 EUR.","kind":"observation","cells":["metric:bookings_won"]}`
	for text, refused := range map[string]bool{
		"The pipeline rests on d1.":            true,
		"The pipeline rests on Tier1 support.": false,
	} {
		reply := `{"sentences":[` + won + `,{"text":"` + text + `","cells":["metric:open_pipeline"]}],"next_decision":null}`
		if _, err := Parse(reply, decisionInput()); (err != nil) != refused {
			t.Errorf("%q: refused = %v, want %v (%v)", text, err != nil, refused, err)
		}
	}
}

func TestParseKeepsADecisionQuotingItsDealsAmount(t *testing.T) {
	won := `{"sentences":[{"text":"Nothing is won yet at 0 EUR.","kind":"observation","cells":["metric:bookings_won"]}],`
	written, err := parseCited(won+`"next_decision":{"deal_id":"deal-globex","text":"Decide who signs off the 25000 EUR renewal."}}`, decisionInput())
	if err != nil {
		t.Fatal(err)
	}
	if written.Next == nil || !strings.Contains(written.Next.Text, "25,000 EUR") {
		t.Fatalf("the decision was %+v, want it kept with the deal's display amount", written.Next)
	}
}

func TestADealQuantityStandsOnlyForTheAmountOfADealTheSentenceNames(t *testing.T) {
	cited := `,"cells":["metric:open_pipeline"],"records":["decision-1","deal-globex"]}`
	for text, kept := range map[string]bool{
		"The Globex renewal at {{deal-globex}} carries most of the pipeline.":                     true,
		"You have an open decision regarding {{deal-globex}} valued at {{deal-globex}}.":          false,
		"The open pipeline also includes {{deal-globex}}.":                                        false,
		"The Globex renewal at {{deal-globex}} is worth {{deal-globex}}.":                         false,
		"The Globex renewal still owes the decision to {{decision-1}}.":                           false,
		"The Globex renewal at {{deal-globex}} sits inside {{metric:open_pipeline}} of pipeline.": true,
	} {
		written, err := parseCited(`{"sentences":[{"text":"Nothing is won yet at 0 EUR.","cells":["metric:bookings_won"]},{"text":"`+text+`"`+cited+`],"next_decision":null}`, decisionInput())
		if err != nil {
			t.Fatalf("%q: %v", text, err)
		}
		if got := len(written.Sentences) == 2; got != kept {
			t.Errorf("%q: kept %v, want %v", text, got, kept)
		}
	}
	written, err := parseCited(`{"sentences":[{"text":"Nothing is won yet at 0 EUR.","cells":["metric:bookings_won"]}],"next_decision":{"deal_id":"deal-globex","text":"Ask {{deal-globex}} for the budget owner."}}`, decisionInput())
	if err != nil {
		t.Fatal(err)
	}
	if written.Next != nil {
		t.Fatalf("a decision printing the amount where the deal's name belongs was kept: %+v", written.Next)
	}
}

// parseCited parses a reply scripted with the input's record ids as the model
// would write it. Each id is spelled as the ref the request showed for it.
func parseCited(reply string, in Input) (Written, error) {
	_, refs := asShown(in)
	var pairs []string
	for _, candidate := range in.Candidates {
		pairs = append(pairs, candidate.ID, refs.deals.For(candidate.ID), candidate.OwnerID, refs.owners.For(candidate.OwnerID))
	}
	for _, decision := range in.Decisions {
		pairs = append(pairs, decision.ID, refs.decisions.For(decision.ID))
	}
	return Parse(strings.NewReplacer(pairs...).Replace(reply), in)
}

// The request shows each deal, decision and owner by its ref; the reply's refs
// read back to the next decision's deal and owner. The ids the request never
// showed, and a ref it never minted, name nothing.
func TestTheAssessmentNamesRecordsByTheRefsItShowed(t *testing.T) {
	in := decisionInput()
	shown := Request(in, "en").Messages[0].Content
	for _, id := range []string{"deal-globex", "decision-1", "user-1"} {
		if strings.Contains(shown, id) {
			t.Errorf("the request shows the record id %s", id)
		}
	}
	won := `{"sentences":[{"text":"Nothing is won yet at 0 EUR.","cells":["metric:bookings_won"]}],`
	for deal, want := range map[string]string{"d1": "deal-globex", "deal-globex": "", "d2": ""} {
		written, err := Parse(won+`"next_decision":{"deal_id":"`+deal+`","owner_id":"u1","text":"Decide who signs off."}}`, in)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if written.Next != nil {
			got = written.Next.DealID + "/" + written.Next.OwnerID
		}
		if wantGot := want + "/user-1"; (want == "" && got != "") || (want != "" && got != wantGot) {
			t.Errorf("a next decision on %q read back as %q, want %q", deal, got, want)
		}
	}
	if _, err := Parse(won[:len(won)-2]+`,{"text":"The renewal d1 is open.","cells":["metric:bookings_won"]}],"next_decision":null}`, in); err == nil {
		t.Error("a reply writing a ref into reader text was kept")
	}
}
