// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

// An external test because it reads the committed corpus through aicert's own
// loader, and aicert imports compose.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
)

// assertEveryScenarioHasADecisionForm prepares every corpus scenario of task
// and asserts each is a decision case at its registered site, whose own
// expected answer, kept, grades as accepted and whose other answers do not.
func assertEveryScenarioHasADecisionForm(t *testing.T, task ai.Task) {
	t.Helper()
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := aicert.LoadCorpus("aicert/corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	prepared := 0
	for _, sc := range scenarios {
		if ai.Task(sc.Task) != task {
			continue
		}
		factory, bound := census.CaseFor(ai.Task(sc.Task), sc.Site)
		if !bound {
			t.Fatalf("scenario %s names site %s/%s, which no case is bound to", sc.Name, sc.Task, sc.Site)
		}
		pc, err := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
		if err != nil {
			t.Fatalf("scenario %s: %v", sc.Name, err)
		}
		dc, ok := pc.(aitasks.DecisionCase)
		if !ok {
			t.Fatalf("scenario %s: the %s case has no decision form, so it cannot be certified on a decision lane", sc.Name, task)
		}
		if dc.DecisionSite() != factory.Site().Variant {
			t.Errorf("scenario %s: decision site %q, want the registered variant %q", sc.Name, dc.DecisionSite(), factory.Site().Variant)
		}
		if len(dc.DecisionRequest().State) == 0 {
			t.Errorf("scenario %s: the decision request carries no state", sc.Name)
		}
		var expected string
		if err := json.Unmarshal(sc.Expect.Answer, &expected); err != nil {
			t.Fatalf("scenario %s: the expected answer is not a label: %v", sc.Name, err)
		}
		if got := dc.EvaluateDecision(decision.Answer{Choice: expected, Confidence: 1}); got.Result != aitasks.OutcomeAccepted {
			t.Errorf("scenario %s: its own expected answer grades %s: %s", sc.Name, got.Result, got.Detail)
		}
		for label := range dc.Floors() {
			if label == expected {
				continue
			}
			if got := dc.EvaluateDecision(decision.Answer{Choice: label, Confidence: 1}); got.Result != aitasks.OutcomeWrongAnswer {
				t.Errorf("scenario %s: answer %q where %q is expected grades %s, want wrong_answer", sc.Name, label, expected, got.Result)
			}
		}
		prepared++
	}
	if prepared == 0 {
		t.Fatalf("no %s scenario was prepared, so nothing about its decision form was measured", task)
	}
}

func TestTheCounterpartyCaseHasADecisionForm(t *testing.T) {
	assertEveryScenarioHasADecisionForm(t, ai.TaskCaptureCounterpartyVerdict)
}
