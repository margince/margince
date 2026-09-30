// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

// A task's `decision:` declaration and its certification case's decision form
// are two claims about one fact: that this task's site asks a decision model
// first. The declaration is what routing reads to offer the lane, and the
// adapter is what certification measures, so each without the other is a lane
// nobody can certify or a certified form nobody routes to. An external test
// because it reads the committed corpus through aicert's own loader, and aicert
// imports compose.

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

func TestEveryDecisionTaskHasAnAdapterAndEveryAdapterIsDeclared(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := aicert.LoadCorpus("aicert/corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	adapted := decisionAdaptedTasks(t, census, scenarios)
	declared := ai.DecisionTasks()
	if len(declared) == 0 {
		t.Fatal("no task declares a decision form, so this census compares two empty sets and measures nothing")
	}
	for _, task := range declared {
		if !slices.Contains(adapted, task) {
			t.Errorf("task %s declares `decision:` and no certification case of it has a decision form — the lane would be offered for a site nobody can certify on it", task)
		}
	}
	for _, task := range adapted {
		if !slices.Contains(declared, task) {
			t.Errorf("task %s has a certification case with a decision form and does not declare `decision:` — the form is measured and routing never offers it", task)
		}
	}
}

// decisionAdaptedTasks prepares every corpus scenario of every registered site
// and names the tasks whose case has a decision form. Every scenario, not the
// first: a case that has the form for one fixture and not another is two
// answers to one question, and is refused.
func decisionAdaptedTasks(t *testing.T, census *aitasks.Registry, scenarios []aicert.Scenario) []ai.Task {
	t.Helper()
	var adapted []ai.Task
	for _, site := range census.All() {
		factory, bound := census.CaseFor(site.Task, site.Variant)
		if !bound {
			// Validate refuses an unbound shipped site; one still unbound is a
			// planned site, with no case to have a form.
			continue
		}
		prepared, withForm := 0, 0
		for _, sc := range scenarios {
			if ai.Task(sc.Task) != site.Task || sc.Site != site.Variant {
				continue
			}
			pc, err := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
			if err != nil {
				t.Fatalf("scenario %s: preparing its case: %v", sc.Name, err)
			}
			prepared++
			if dc, ok := pc.(aitasks.DecisionCase); ok {
				withForm++
				if dc.DecisionSite() != site.Variant {
					t.Errorf("scenario %s: decision asked at %q, want the registered variant %q", sc.Name, dc.DecisionSite(), site.Variant)
				}
			}
		}
		if prepared == 0 {
			// Under-recognition guard: a site with no scenario is a site this
			// census cannot read, and would otherwise count as having no form.
			t.Errorf("site %s/%s has a certification case and no corpus scenario, so whether it has a decision form is unmeasured", site.Task, site.Variant)
			continue
		}
		if withForm != 0 && withForm != prepared {
			t.Errorf("site %s/%s has a decision form for %d of %d scenarios; a site asks a decision model first or it does not", site.Task, site.Variant, withForm, prepared)
		}
		if withForm > 0 && !slices.Contains(adapted, site.Task) {
			adapted = append(adapted, site.Task)
		}
	}
	return adapted
}
