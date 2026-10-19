// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

func TestTranscriptCertificationGradesCompletedWorkAsNonAction(t *testing.T) {
	c := transcriptProposeCase{lines: []string{"Mira: I sent the revised scope."}, expected: []transcriptProposeExpectation{{Line: 1, Optional: true, Meaning: &commitmentMeaning{State: "completed", DeadlinePrecision: "none"}}}}
	for _, state := range []string{"completed", "open", "unclear", "request"} {
		trace := aitasks.Trace{Output: fmt.Sprintf(`{"proposals":[{"summary":"Send scope","owner":"Mira","promiser_ref":"","due_date":"","source_lines":[1],"confidence":0.99,"commitment":{"state":%q,"condition":"","deadline_precision":"none","deadline_quote":""}}]}`, state)}
		outcome := c.Evaluate(trace)
		if (outcome.Result == aitasks.OutcomeAccepted) != (state == "completed") {
			t.Fatalf("%s: %+v", state, outcome)
		}
	}
}

func TestMailCertificationGradesCompletedWorkAsNonAction(t *testing.T) {
	prepared, err := (signalExtractCases{}).Prepare(json.RawMessage(`[{"direction":"outbound","body":"I sent the revised scope."}]`), json.RawMessage(`[{"kind":"commitment_made","message":1,"optional":true,"meaning":{"state":"completed","condition":"","deadline_precision":"none","deadline_quote":""}}]`))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := prepared.(*signalExtractCase)
	if !ok {
		t.Fatal("unexpected signal case")
	}
	for _, state := range []string{"completed", "open", "unclear", "request"} {
		trace := aitasks.Trace{Output: fmt.Sprintf(`{"events":[{"kind":"commitment_made","message_id":%q,"summary":"Send scope","quote":"I sent the revised scope.","due_date":"","promiser_ref":"","confidence":0.99,"commitment":{"state":%q,"condition":"","deadline_precision":"none","deadline_quote":""}}]}`, c.thread.Messages[0].ID.String(), state)}
		outcome := c.Evaluate(trace)
		if (outcome.Result == aitasks.OutcomeAccepted) != (state == "completed") {
			t.Fatalf("%s: %+v", state, outcome)
		}
	}
}

func TestMailCertificationDoesNotInventADayFromARange(t *testing.T) {
	prepared, err := (signalExtractCases{}).Prepare(json.RawMessage(`[{"direction":"outbound","body":"I will send the scope next week."}]`), json.RawMessage(`[{"kind":"commitment_made","message":1,"due_date":""}]`))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := prepared.(*signalExtractCase)
	if !ok {
		t.Fatal("unexpected signal case")
	}
	for _, due := range []string{"", "2026-09-14"} {
		precision := "range"
		if due != "" {
			precision = "day"
		}
		trace := aitasks.Trace{Output: fmt.Sprintf(`{"events":[{"kind":"commitment_made","message_id":%q,"summary":"Send scope","quote":"I will send the scope next week.","due_date":%q,"promiser_ref":"","confidence":0.99,"commitment":{"state":"open","condition":"","deadline_precision":%q,"deadline_quote":"next week"}}]}`, c.thread.Messages[0].ID.String(), due, precision)}
		outcome := c.Evaluate(trace)
		if (outcome.Result == aitasks.OutcomeAccepted) != (due == "") {
			t.Fatalf("date %q: %+v", due, outcome)
		}
	}
}

func TestMeaningExpectationsCannotMakeOpenWorkOptional(t *testing.T) {
	for _, expected := range []string{
		`[{"line":1,"optional":true}]`,
		`[{"line":1,"optional":true,"meaning":{"state":"open","deadline_precision":"none"}}]`,
	} {
		if _, err := (transcriptProposeCases{}).Prepare(json.RawMessage(`["Mira: I will send pricing."]`), json.RawMessage(expected)); err == nil {
			t.Fatalf("unsafe expectation accepted: %s", expected)
		}
	}
}

func TestMailDeadlineMustComeFromThePromisedSentence(t *testing.T) {
	prepared, err := (signalExtractCases{}).Prepare(json.RawMessage(`[{"direction":"outbound","body":"I will send pricing next week. The meeting is Friday."}]`), json.RawMessage(`[{"kind":"commitment_made","message":1,"due_date":""}]`))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := prepared.(*signalExtractCase)
	if !ok {
		t.Fatal("unexpected case")
	}
	trace := aitasks.Trace{Output: fmt.Sprintf(`{"events":[{"kind":"commitment_made","message_id":%q,"summary":"Send pricing","quote":"I will send pricing next week.","due_date":"2026-09-11","promiser_ref":"","confidence":0.99,"commitment":{"state":"open","condition":"","deadline_precision":"day","deadline_quote":"Friday"}}]}`, c.thread.Messages[0].ID.String())}
	if got := c.Evaluate(trace); got.Result != aitasks.OutcomeInvalid {
		t.Fatalf("unrelated deadline accepted: %+v", got)
	}
}

func TestMeaningQuotesKeepRequiredEvidenceWithinTheirSourceWords(t *testing.T) {
	want := &commitmentMeaning{State: "conditional", Condition: "the budget is approved", DeadlinePrecision: "range", DeadlineQuote: "next week"}
	for _, tc := range []struct {
		name     string
		meaning  commitmentMeaning
		accepted bool
	}{
		{"whole conditional clause", commitmentMeaning{State: "conditional", Condition: "If the budget is approved", DeadlinePrecision: "range", DeadlineQuote: "during next week"}, true},
		{"missing part of condition", commitmentMeaning{State: "conditional", Condition: "the budget", DeadlinePrecision: "range", DeadlineQuote: "next week"}, false},
		{"different condition", commitmentMeaning{State: "conditional", Condition: "the scope is approved", DeadlinePrecision: "range", DeadlineQuote: "next week"}, false},
		{"different range", commitmentMeaning{State: "conditional", Condition: "the budget is approved", DeadlinePrecision: "range", DeadlineQuote: "next month"}, false},
		{"word fragment", commitmentMeaning{State: "conditional", Condition: "the budget is approved", DeadlinePrecision: "range", DeadlineQuote: "next weekend"}, false},
		{"wrong state", commitmentMeaning{State: "open", Condition: "the budget is approved", DeadlinePrecision: "range", DeadlineQuote: "next week"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := expectedMeaningMatches(want, &tc.meaning); got != tc.accepted {
				t.Fatalf("matches=%t want %t", got, tc.accepted)
			}
		})
	}
}

func TestTranscriptCertificationAcceptsQuoteExpansionButNotInventedWords(t *testing.T) {
	c := transcriptProposeCase{lines: []string{"Mira: If the budget is approved, I will send the draft during next week."}, expected: []transcriptProposeExpectation{{Line: 1, Optional: true, Meaning: &commitmentMeaning{State: "conditional", Condition: "the budget is approved", DeadlinePrecision: "range", DeadlineQuote: "next week"}}}}
	for _, condition := range []string{"If the budget is approved", "When the budget is approved"} {
		trace := aitasks.Trace{Output: fmt.Sprintf(`{"proposals":[{"summary":"Send draft","owner":"Mira","promiser_ref":"","due_date":"","source_lines":[1],"confidence":0.9,"commitment":{"state":"conditional","condition":%q,"deadline_precision":"range","deadline_quote":"during next week"}}]}`, condition)}
		outcome := c.Evaluate(trace)
		if (outcome.Result == aitasks.OutcomeAccepted) != (condition == "If the budget is approved") {
			t.Fatalf("%s: %+v", condition, outcome)
		}
	}
}
