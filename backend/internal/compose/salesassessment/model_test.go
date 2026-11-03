// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

import (
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
	written, err := Parse(`{"sentences":[{"text":"Sales won stand at 216000 EUR.","kind":"observation","cells":["metric:bookings_won"]}],"next_decision":{"deal_id":"deal-1","text":"Offer a 15 percent discount."},"confidence":"low"}`, assessmentInput())
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
	written, err := Parse(kept, assessmentInput())
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
		written, err := Parse(reply, assessmentInput())
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
