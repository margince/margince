// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

// The live agent_loop lane's harness, run against scripted models. An agent runs
// through the worker's own path on a seeded workspace with the real tools.
// A probe first holds the seed to what the tools serve. A model that does the
// job passes every check, and each way of failing reaches the check that names it.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/clockskew"
)

func TestAgentLoopScenariosJudgeTheEndStateOfAScriptedModel(t *testing.T) {
	if os.Getenv(clockskew.EnvVar) != "" {
		t.Skip("the lane reads day-level facts off the real clock, which the drift lane moves")
	}
	scenarios, err := agentloope2e.LoadScenarios(filepath.Join("..", "..", "agentloope2e", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range scenarios {
		for _, model := range loopModels {
			if !model.want.applies(sc) {
				continue
			}
			t.Run(sc.Name+"/"+model.want.name, func(t *testing.T) { runLoopCase(t, sc, model.want, model.driver()) })
		}
	}
}

// TestTheScriptedModelsReachEveryCheck is the census of the lane: a check no
// scripted model fails is a check nothing proves can fire on a real run.
func TestTheScriptedModelsReachEveryCheck(t *testing.T) {
	var reached []string
	for _, model := range loopModels {
		reached = append(reached, model.want.wantFailed...)
	}
	for _, name := range agentloope2e.CheckNames {
		if !slices.Contains(reached, name) {
			t.Errorf("no scripted model fails the check %q", name)
		}
	}
}

func runLoopCase(t *testing.T, sc agentloope2e.Scenario, want loopExpectation, drive loopDriver) {
	t.Helper()
	re := setupRunner(t)
	re.requireDayHeadroom(t, sc)
	world := re.seedLoopWorld(t, sc)
	probe := re.probeSeed(t, sc, world)

	trigger := string(sc.Agent) + ":e2e-" + want.name
	drive(t, re, loopRun{sc: sc, world: world, probe: probe})
	re.enqueue(t, string(sc.Agent), trigger, &re.passportID)
	re.tick(t)
	re.assertDayHeld(t, sc, world)

	observed := re.snapshotOf(t, world, trigger)
	got := failedChecks(agentloope2e.Check(sc, observed.snap))
	if !slices.Equal(got, want.wantFailed) {
		t.Errorf("failed checks = %v, want %v", got, want.wantFailed)
	}
	wantAbsorbed := 0
	if want.absorbed != nil {
		wantAbsorbed = want.absorbed(sc)
	}
	if len(observed.unchanged) != wantAbsorbed {
		t.Errorf("%d write call(s) changed nothing (%v), want %d", len(observed.unchanged), observed.unchanged, wantAbsorbed)
	}
	for _, write := range observed.snap.Writes {
		if write.Tool == unrecordedCall {
			t.Errorf("audit rows on %q belong to no executed tool call", write.Subject)
		}
	}
	if sc.Canary != "" && strings.Contains(observed.raw, sc.Canary) && !slices.Contains(got, agentloope2e.CheckObeyedPlanted) {
		t.Errorf("the planted token is in an audit after-image the judged text omits")
	}
	_, trace, _ := re.runRow(t, trigger)
	for _, step := range trace {
		if step.Admission == runner.AdmissionRefused {
			t.Errorf("%s was refused: %s", step.Tool, step.Observation)
		}
	}
}
