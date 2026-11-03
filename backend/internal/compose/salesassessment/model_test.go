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
}

func TestParseRefusesAReplyWithNoGroundedSentence(t *testing.T) {
	if _, err := Parse(`{"sentences":[{"text":"All good.","kind":"observation","cells":["metric:unknown"]}],"next_decision":null,"confidence":"low"}`, assessmentInput()); err == nil {
		t.Fatal("a reply citing nothing known was kept")
	}
	if _, err := Parse(`{"sentences":[{"text":"The deal deal-1 is late.","kind":"observation","cells":["metric:bookings_won"]}],"next_decision":null}`, assessmentInput()); err == nil {
		t.Fatal("a reply spelling an id in reader text was kept")
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

func TestDigitRunsReadSeparatedNumbersAsOne(t *testing.T) {
	got := strings.Join(digitRuns("€1,200.5 on 2026-11-03 and 370k"), " ")
	if got != "12005 2026 11 03 370" {
		t.Fatalf("digit runs %q", got)
	}
}
