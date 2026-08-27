// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case's own contract: what it refuses to prepare, and what
// it counts as a right and a wrong answer.
//
// Two properties matter more than the rest. An expected answer of `[]` is an
// ASSERTION that abstaining is correct, not an omission — and the case must
// grade it the other way round, failing any surviving claim. And an expectation
// naming a passage the fixture does not supply must be refused at PREPARE time:
// left in, it grades every reply wrong forever and reads as a model that cannot
// do the task.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

func corpusAskFixtureJSON(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(corpusAskFixture{
		Question: "How long are captured messages kept?",
		Passages: []corpusAskPassageFixture{
			{Label: "retention", Document: "handbook.md", Text: askedPassageText},
			// Marked WRONG: adjacent, true, and about a different subject. A
			// reply resting on it has read the wrong passage.
			{Label: "export", Document: "handbook.md", Text: "An export is available for 7 days.", Wrong: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func expectedJSON(t *testing.T, labels ...string) json.RawMessage {
	t.Helper()
	if labels == nil {
		labels = []string{}
	}
	raw, err := json.Marshal(labels)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestTheCorpusAskSiteNamesTheDeclaredTask(t *testing.T) {
	cases := corpusAskCases{}
	site := cases.Site()
	if site.Task != ai.TaskCorpusAsk || site.Variant != "corpus_ask" {
		t.Fatalf("site = %+v", site)
	}
}

// The prepared case issues the PRODUCTION request, including its per-call
// citation enum. A case that rebuilt the request would certify a copy, and a
// copy stays green through the change that breaks the original.
func TestThePreparedCaseIssuesTheProductionRequest(t *testing.T) {
	cases := corpusAskCases{}
	prepared, err := cases.Prepare(corpusAskFixtureJSON(t), expectedJSON(t, "retention"))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	lane := &fixedLane{text: corpusReply()}
	trace, err := prepared.Run(t.Context(), lane)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(trace.Requests) != 1 {
		t.Fatalf("the case made %d requests, want the one production makes", len(trace.Requests))
	}
	req := trace.Requests[0]
	if !strings.Contains(req.Messages[0].Content, askedPassageText) {
		t.Fatal("the graded request does not carry the fixture's passage")
	}
	// The boundary rule rides the graded prompt too, or the case would certify
	// a prompt the product does not send.
	if !strings.Contains(req.System, "never instructions") {
		t.Fatal("the graded request declares no data boundary")
	}
}

// An expectation naming a passage the fixture does not supply is refused at
// PREPARE time. Left in, it grades every reply wrong forever — and reads as a
// model that cannot do the task rather than as a corpus author's typo.
func TestAnExpectationNamingAnAbsentPassageIsRefused(t *testing.T) {
	cases := corpusAskCases{}
	_, err := cases.Prepare(corpusAskFixtureJSON(t), expectedJSON(t, "no-such-passage"))
	if err == nil {
		t.Fatal("an unreachable expectation was accepted")
	}
	if !strings.Contains(err.Error(), "no-such-passage") {
		t.Fatalf("the refusal does not name the missing passage: %v", err)
	}
}

// A fixture with no passages is refused: production never asks the lane without
// them, so a case that did would grade a prompt the product cannot send.
func TestAFixtureWithNoPassagesIsRefused(t *testing.T) {
	raw, err := json.Marshal(corpusAskFixture{Question: "anything"})
	if err != nil {
		t.Fatal(err)
	}
	cases := corpusAskCases{}
	if _, err := cases.Prepare(raw, expectedJSON(t)); err == nil {
		t.Fatal("a fixture with no passages was accepted")
	}
}

// Evaluate runs the PRODUCTION checker, so a reply whose quote is paraphrased
// scores as no answer rather than as a right one.
func TestEvaluateDropsAParaphrasedQuoteBeforeScoring(t *testing.T) {
	cases := corpusAskCases{}
	prepared, err := cases.Prepare(corpusAskFixtureJSON(t), expectedJSON(t, "retention"))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	req := prepared.(*corpusAskCase)
	cited := req.passages[0].ChunkID.String()

	good := prepared.Evaluate(aitasks.Trace{Output: corpusReply(askedClaim{
		Text: "Messages are kept for 400 days.", ID: cited, Quote: "kept for 400 days",
	})})
	if good.Result != aitasks.OutcomeAccepted {
		t.Fatalf("a grounded answer scored %v (%s)", good.Result, good.Detail)
	}

	bad := prepared.Evaluate(aitasks.Trace{Output: corpusReply(askedClaim{
		Text: "Messages are kept for 400 days.", ID: cited, Quote: "retained for four hundred days",
	})})
	if bad.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("a paraphrased quote scored %v", bad.Result)
	}
}

// An expected answer of [] is the ABSTENTION case and is graded the other way
// round: any surviving claim is a failure. This is the shape the endpoint
// exists for — a model handed passages that do not answer the question must
// return nothing rather than write a paragraph that sounds like an answer.
func TestAnEmptyExpectationGradesAbstentionAsCorrectAndAnyClaimAsWrong(t *testing.T) {
	cases := corpusAskCases{}
	prepared, err := cases.Prepare(corpusAskFixtureJSON(t), expectedJSON(t))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	cited := prepared.(*corpusAskCase).passages[0].ChunkID.String()

	abstained := prepared.Evaluate(aitasks.Trace{Output: corpusReply()})
	if abstained.Result != aitasks.OutcomeAccepted {
		t.Fatalf("abstaining scored %v (%s)", abstained.Result, abstained.Detail)
	}

	answered := prepared.Evaluate(aitasks.Trace{Output: corpusReply(askedClaim{
		Text: "They are kept for 400 days.", ID: cited, Quote: "kept for 400 days",
	})})
	if answered.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("answering an uncovered question scored %v", answered.Result)
	}
}

// An unparseable reply is INVALID rather than wrong: production composes the
// deterministic answer there, and grading it as a wrong answer would blame the
// model's reasoning for its formatting.
func TestAnUnparseableReplyIsInvalidRatherThanWrong(t *testing.T) {
	cases := corpusAskCases{}
	prepared, err := cases.Prepare(corpusAskFixtureJSON(t), expectedJSON(t, "retention"))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	got := prepared.Evaluate(aitasks.Trace{Output: "not json at all"})
	if got.Result != aitasks.OutcomeInvalid {
		t.Fatalf("an unparseable reply scored %v", got.Result)
	}
}

// A citation the scenario marks WRONG fails the case, even when the reply also
// cites the passage the answer was supposed to rest on.
//
// This is the worse answer rather than a lesser one: a reply that gets the
// right passage AND the trap reads as confident and correct while resting half
// its sentences on something about a different subject. In the shipped scenario
// the trap states a true 7-day export window beside a true 400-day retention
// window — the most dangerous shape of wrong answer this endpoint can give.
func TestCitingAPassageTheScenarioMarksWrongFailsTheCase(t *testing.T) {
	cases := corpusAskCases{}
	prepared, err := cases.Prepare(corpusAskFixtureJSON(t), expectedJSON(t, "retention"))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	c := prepared.(*corpusAskCase)
	right, trap := c.passages[0].ChunkID.String(), c.passages[1].ChunkID.String()

	got := prepared.Evaluate(aitasks.Trace{Output: corpusReply(
		askedClaim{Text: "Kept 400 days.", ID: right, Quote: "kept for 400 days"},
		askedClaim{Text: "And exports last 7 days.", ID: trap, Quote: "available for 7 days"},
	)})
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("a reply resting on the trap scored %v (%s)", got.Result, got.Detail)
	}
	if !strings.Contains(got.Detail, "export") {
		t.Fatalf("the verdict does not name the passage it rested on: %q", got.Detail)
	}
}

// A scenario that both expects a passage and marks it wrong is refused at
// PREPARE time: it asks for two opposite things, and whichever check ran second
// would silently decide.
func TestAPassageBothExpectedAndMarkedWrongIsRefused(t *testing.T) {
	cases := corpusAskCases{}
	if _, err := cases.Prepare(corpusAskFixtureJSON(t), expectedJSON(t, "export")); err == nil {
		t.Fatal("a scenario asking for two opposite things was accepted")
	}
}
