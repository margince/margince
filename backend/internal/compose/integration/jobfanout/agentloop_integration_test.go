// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

// The live agent_loop lane's harness, run against scripted models. An agent runs
// through the worker's own path on a seeded workspace with the real tools.
// A model that does the job passes every check. Each way of failing reaches
// the check that names it.

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
)

func TestAgentLoopScenariosJudgeTheEndStateOfAScriptedModel(t *testing.T) {
	scenarios, err := agentloope2e.LoadScenarios(filepath.Join("..", "..", "agentloope2e", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range scenarios {
		for _, model := range loopModels {
			if !model.applies(sc) {
				continue
			}
			t.Run(sc.Name+"/"+model.name, func(t *testing.T) { runLoopCase(t, sc, model) })
		}
	}
}

func runLoopCase(t *testing.T, sc agentloope2e.Scenario, model loopModel) {
	t.Helper()
	re := setupRunner(t)
	world := re.seedLoopWorld(t, sc)
	trigger := string(sc.Agent) + ":e2e-" + model.name
	re.brain.Script(model.script(t, sc, world)...)
	re.enqueue(t, string(sc.Agent), trigger, &re.passportID)
	re.tick(t)
	if sc.Agent == agentloope2e.MorningBrief {
		re.assertBriefDayHeld(t, world)
	}

	got := failedChecks(agentloope2e.Check(sc, re.snapshotOf(t, world, trigger)))
	if !slices.Equal(got, model.wantFailed) {
		t.Errorf("failed checks = %v, want %v", got, model.wantFailed)
	}
	if model.wantFailed == nil {
		re.assertRunReachedItsEvidence(t, sc, world, trigger)
	}
}

// assertRunReachedItsEvidence holds the seed to what the scenario relies on.
// No tool refused a call, a real read returned the planted text, and the sweep's
// list held its worklist and no forbidden deal. A seed that stops planting its
// text would otherwise pass every check for the wrong reason.
func (re *runnerEnv) assertRunReachedItsEvidence(t *testing.T, sc agentloope2e.Scenario, w *loopWorld, trigger string) {
	t.Helper()
	_, trace, _ := re.runRow(t, trigger)
	planted := false
	for _, step := range trace {
		if strings.Contains(step.Observation, "refused") {
			t.Errorf("%s was refused: %s", step.Tool, step.Observation)
		}
		if sc.Canary != "" && step.Tool == "read_record" && strings.Contains(step.Observation, sc.Canary) {
			planted = true
		}
		if step.Tool != "whats_slipping_this_week" {
			continue
		}
		for _, name := range sc.Worklist {
			if !strings.Contains(step.Observation, w.deals[name]) {
				t.Errorf("the slipping list does not hold %s", name)
			}
		}
		for _, name := range sc.Forbidden {
			if strings.Contains(step.Observation, w.deals[name]) {
				t.Errorf("the slipping list holds %s, which the run must not touch", name)
			}
		}
	}
	if sc.Canary != "" && !planted {
		t.Errorf("no real read returned the planted text %q", sc.Canary)
	}
}
