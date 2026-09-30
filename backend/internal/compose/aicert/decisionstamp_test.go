// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// decisionStampOf is the one stand-in scenario's decision stamp under form.
func decisionStampOf(t *testing.T, form widgetForm) string {
	t.Helper()
	sc := decidingScenario("basic", ai.TaskSiteTriage)
	stamps, err := DecisionScenarioStamps([]Scenario{sc}, decidingCensus(t, ai.TaskSiteTriage, form))
	if err != nil {
		t.Fatalf("DecisionScenarioStamps: %v", err)
	}
	stamp, ok := stamps[sc.Name]
	if !ok {
		t.Fatalf("a scenario whose case has a decision form got no decision stamp: %v", stamps)
	}
	return stamp
}

// The decision stamp follows the decision form — its criteria and the floors
// the gate keeps an answer at — and nothing the LLM prompt says. Moving the
// LLM prompt must not stale a decision record, and moving a criterion must.
func TestTheDecisionStampMovesWithACriterionAndAFloorButNotWithTheLLMPrompt(t *testing.T) {
	base := defaultWidgetForm()
	was := decisionStampOf(t, base)

	criterion := base
	criterion.criterion = "the subject is a widget, and says so"
	if got := decisionStampOf(t, criterion); got == was {
		t.Error("editing a criterion left the decision stamp where it was")
	} else if parts := splitStampChange(was, got); !parts.Prompt || parts.Case || parts.Grader {
		t.Errorf("a criterion edit reads as %q, want the prompt half alone", parts.Describe())
	}

	floor := base
	floor.floor = 0.85
	if decisionStampOf(t, floor) == was {
		t.Error("moving a floor left the decision stamp where it was")
	}

	prompt := base
	prompt.system = "Describe the subject in two sentences."
	if decisionStampOf(t, prompt) != was {
		t.Error("editing the LLM prompt moved the decision stamp; a decision record would go stale over a prompt it never sent")
	}
}

// The LLM stamp is blind to the decision form: a case that gains one keeps the
// stamp every committed completion record was scored under.
func TestTheLLMStampIgnoresTheDecisionForm(t *testing.T) {
	sc := decidingScenario("basic", ai.TaskSummarize)
	plain, err := ScenarioStamps(context.Background(), []Scenario{sc}, testCensus(t))
	if err != nil {
		t.Fatalf("ScenarioStamps over the plain case: %v", err)
	}
	deciding, err := ScenarioStamps(context.Background(), []Scenario{sc}, decidingCensus(t, ai.TaskSummarize, defaultWidgetForm()))
	if err != nil {
		t.Fatalf("ScenarioStamps over the deciding case: %v", err)
	}
	if plain[sc.Name] != deciding[sc.Name] {
		t.Error("a case gaining a decision form moved its LLM stamp; every committed record for the site would go stale")
	}
}

// A scenario whose case has no decision form has no decision stamp, and a case
// that asks its decision at another site than the scenario runs on is refused.
func TestOnlyADecisionCaseIsDecisionStamped(t *testing.T) {
	sc := testScenario("plain", wideBands)
	stamps, err := DecisionScenarioStamps([]Scenario{sc}, testCensus(t))
	if err != nil || len(stamps) != 0 {
		t.Fatalf("a case with no decision form was decision-stamped: %v, %v", stamps, err)
	}

	unbound := decidingScenario("basic", ai.TaskSiteTriage)
	unbound.Site = "not_the_widget"
	_, err = DecisionScenarioStamps([]Scenario{unbound}, decidingCensus(t, ai.TaskSiteTriage, defaultWidgetForm()))
	if err == nil || !strings.Contains(err.Error(), "binds no certification case") {
		t.Fatalf("a scenario on an unbound site: want a refusal naming it, got %v", err)
	}

	elsewhere := defaultWidgetForm()
	elsewhere.askAt = "another_site"
	_, err = DecisionScenarioStamps([]Scenario{decidingScenario("basic", ai.TaskSiteTriage)},
		decidingCensus(t, ai.TaskSiteTriage, elsewhere))
	if err == nil || !strings.Contains(err.Error(), `asks its decision at "another_site"`) {
		t.Fatalf("a case asking its decision at another site: want a refusal naming it, got %v", err)
	}
}
