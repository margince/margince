// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// aitasksTrace is a reply as the cert lane hands one to Evaluate.
func aitasksTrace(output string) aitasks.Trace {
	return aitasks.Trace{Output: output}
}

// fixedCompleter answers every call with the same reply, so a test can watch
// what the site ASKS rather than what a model would answer.
type fixedCompleter struct{ reply string }

func (c fixedCompleter) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: c.reply}, nil
}

func roleFixture(t *testing.T) json.RawMessage {
	t.Helper()
	return json.RawMessage(`{
		"deal": "Retrofit 2026",
		"candidates": [{
			"PersonID": "p-1", "FullName": "Dietmar Rietsch", "Title": "Managing Director",
			"Messages": [{"ActivityID": "a-1", "Subject": "Re: Retrofit",
			              "Body": "I sign off the budget for this, so send it to me directly."}]
		}]
	}`)
}

// The scenario a job title would answer wrongly. An empty expectation is a
// real one here: the pass condition is that nothing survives.
func TestProposeRolesCertCaseScoresRestraintAsTheRightAnswer(t *testing.T) {
	t.Parallel()
	prepared, err := proposeRolesCases{}.Prepare(roleFixture(t), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("refused a scorable scenario: %v", err)
	}
	outcome := prepared.Evaluate(aitasksTrace(`{"proposals":[]}`))
	if outcome.Result != "accepted" {
		t.Fatalf("proposing nothing scored %q: %s", outcome.Result, outcome.Detail)
	}
}

// A role read out of a title is the expensive mistake this site exists to
// avoid, so the case must SCORE it rather than let the gate's silence pass as
// agreement.
func TestProposeRolesCertCaseFailsARoleTheEvidenceDoesNotSupport(t *testing.T) {
	t.Parallel()
	prepared, err := proposeRolesCases{}.Prepare(roleFixture(t), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("refused a scorable scenario: %v", err)
	}
	outcome := prepared.Evaluate(aitasksTrace(`{"proposals":[{
		"person_id":"p-1","role":"economic_buyer",
		"evidence_snippet":"I sign off the budget for this, so send it",
		"source_id":"a-1","confidence":0.9}]}`))
	if outcome.Result != "wrong_answer" {
		t.Fatalf("a role nobody expected scored %q", outcome.Result)
	}
	if !strings.Contains(outcome.Detail, "wanted none") {
		t.Fatalf("the detail does not say what was wrong: %q", outcome.Detail)
	}
}

// The scenario the site IS for: the words carry the role, so it survives.
func TestProposeRolesCertCaseAcceptsAWellEvidencedRole(t *testing.T) {
	t.Parallel()
	prepared, err := proposeRolesCases{}.Prepare(roleFixture(t),
		json.RawMessage(`{"p-1":"economic_buyer"}`))
	if err != nil {
		t.Fatalf("refused a scorable scenario: %v", err)
	}
	outcome := prepared.Evaluate(aitasksTrace(`{"proposals":[{
		"person_id":"p-1","role":"economic_buyer",
		"evidence_snippet":"I sign off the budget for this, so send it",
		"source_id":"a-1","confidence":0.9}]}`))
	if outcome.Result != "accepted" {
		t.Fatalf("the quoted role scored %q: %s", outcome.Result, outcome.Detail)
	}
}

// A candidate with no messages is one the input builder never assembles. A
// fixture supplying a bare name and title would certify a call the product
// does not make — and would score the title-only reading as if it were a real
// scenario.
func TestProposeRolesCertCaseRefusesACandidateWithNoWords(t *testing.T) {
	t.Parallel()
	_, err := proposeRolesCases{}.Prepare(json.RawMessage(`{
		"deal": "Retrofit 2026",
		"candidates": [{"PersonID": "p-2", "FullName": "Ute Sommer",
		                "Title": "Chief Financial Officer", "Messages": []}]}`),
		json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("accepted a candidate the product never assembles")
	}
	if !strings.Contains(err.Error(), "no messages") {
		t.Fatalf("the refusal does not name the reason: %v", err)
	}
}

// The gate refuses a proposal for anybody this call did not offer, so such an
// expectation is unsatisfiable by any model answer — a scenario that can only
// fail measures nothing.
func TestProposeRolesCertCaseRefusesAnExpectationNoAnswerCouldMeet(t *testing.T) {
	t.Parallel()
	_, err := proposeRolesCases{}.Prepare(roleFixture(t),
		json.RawMessage(`{"p-stranger":"champion"}`))
	if err == nil {
		t.Fatal("accepted an expectation the gate refuses whatever the model says")
	}
	if !strings.Contains(err.Error(), "not a candidate") {
		t.Fatalf("the refusal does not name the reason: %v", err)
	}
}

// The site sends the production request, so the fence and every candidate's
// words must be in what it issues — a case building its own prompt would
// certify a prompt nobody serves.
func TestProposeRolesCertCaseIssuesTheProductionRequest(t *testing.T) {
	t.Parallel()
	prepared, err := proposeRolesCases{}.Prepare(roleFixture(t), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("refused a scorable scenario: %v", err)
	}
	trace, err := prepared.Run(t.Context(), fixedCompleter{reply: `{"proposals":[]}`})
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if len(trace.Requests) != 1 {
		t.Fatalf("issued %d requests, want one", len(trace.Requests))
	}
	content := trace.Requests[0].Messages[0].Content
	for _, want := range []string{"Retrofit 2026", "Dietmar Rietsch", "I sign off the budget"} {
		if !strings.Contains(content, want) {
			t.Fatalf("the issued request does not carry %q", want)
		}
	}
}
