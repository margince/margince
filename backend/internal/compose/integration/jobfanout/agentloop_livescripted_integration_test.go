// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

// The live lane's code path with a scripted model standing in for the route and
// a stubbed judge, proved without a paid call. What these tests run is
// what the paid pass runs: the seed probe, the Tick path, the snapshot, the
// judge's door and the verdict file. Only the model and the judge's answer differ.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/agentloope2e/live"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/clockskew"
)

// stubJudge answers yes to every criterion and copies each prompt it was asked
// into the file it returns.
func stubJudge(t *testing.T, root string) (agentloope2e.CommandJudge, string) {
	t.Helper()
	seen := filepath.Join(t.TempDir(), "judge-prompts.txt")
	command := "cmd:tee -a " + seen + " >/dev/null; printf '{\"reason\": \"stub\", \"verdict\": \"yes\"}'"
	return agentloope2e.CommandJudge{Root: root, Env: append(os.Environ(), "E2E_LLM_JUDGE="+command)}, seen
}

func scriptedLane(t *testing.T, name string, runs int) (liveLane, agentloope2e.Scenario, string, string) {
	t.Helper()
	if os.Getenv(clockskew.EnvVar) != "" {
		t.Skip("the lane reads day-level facts off the real clock, which the drift lane moves")
	}
	root := integration.RepositoryRoot(t)
	scenarios, err := agentloope2e.LoadScenarios(filepath.Join(root, live.ScenarioDir))
	if err != nil {
		t.Fatal(err)
	}
	var sc agentloope2e.Scenario
	for _, candidate := range scenarios {
		if candidate.Name == name {
			sc = candidate
		}
	}
	if sc.Name == "" {
		t.Fatalf("no scenario %s", name)
	}
	judge, seen := stubJudge(t, root)
	verdicts := t.TempDir()
	lane := liveLane{
		cfg: live.Config{
			Root: root, VerdictDir: verdicts, Judge: judge, Runs: runs,
			Binding: agentloope2e.Binding{Provider: "fake", Model: "scripted", Tier: "premium"},
		},
		setup: func(t *testing.T) (*runnerEnv, *live.Recorder) { return setupRunner(t), nil },
	}
	return lane, sc, verdicts, seen
}

func scriptedModelNamed(t *testing.T, name string) scriptedModel {
	t.Helper()
	i := slices.IndexFunc(loopModels, func(m scriptedModel) bool { return m.want.name == name })
	if i < 0 {
		t.Fatalf("no scripted model %s", name)
	}
	return loopModels[i]
}

func servedBy(model scriptedModel) liveServe {
	return func(t *testing.T, re *runnerEnv, run loopRun) *compose.RunnerService {
		re.brain.Script(model.script(t, run)...)
		return re.svc
	}
}

func TestLiveLaneFilesAPassingVerdictFromAScriptedRoute(t *testing.T) {
	lane, sc, verdicts, seen := scriptedLane(t, "morning_brief_annotates_its_queue_from_the_evidence", 2)
	lane.serve = servedBy(scriptedModelNamed(t, "does_the_job"))
	got, err := lane.certify(t, sc)
	if err != nil {
		t.Fatal(err)
	}
	filed, err := agentloope2e.ReadVerdicts(verdicts)
	if err != nil || len(filed) != 1 {
		t.Fatalf("read back %d verdict(s), %v; want one", len(filed), err)
	}
	if filed[0].Stamp != got.Stamp || !filed[0].Outcome.Pass || !filed[0].Outcome.Smoke || len(filed[0].Runs) != 2 {
		t.Errorf("filed verdict = %+v", filed[0].Outcome)
	}
	for i, run := range filed[0].Runs {
		if !run.ChecksPassed() || !run.JudgePassed() || len(run.Criteria) != len(sc.Criteria) {
			t.Errorf("run %d = %+v, want every check and every criterion passed", i+1, run)
		}
	}
	if current, err := live.IsCurrent(lane.cfg.Root, verdicts, "", sc, lane.cfg.Binding); err != nil || !current {
		t.Errorf("a verdict just filed is current = %v, %v", current, err)
	}
	prompts, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	judged := string(prompts)
	for _, want := range []string{"WORKSPACE FACTS", "CFO signs off on Thursday", "The latest email on this deal needs a reply"} {
		if !strings.Contains(judged, want) {
			t.Errorf("the judge was not shown %q", want)
		}
	}
	if regexp.MustCompile(`\bdone\b`).MatchString(judged) {
		t.Errorf("the judge was shown the agent's own final summary")
	}
}

func TestLiveLaneFilesAFailingVerdictWithoutAskingTheJudge(t *testing.T) {
	lane, sc, verdicts, seen := scriptedLane(t, "morning_brief_retrieved_text_is_not_an_instruction", 1)
	lane.serve = servedBy(scriptedModelNamed(t, "obeys_the_planted_instruction"))
	if _, err := lane.certify(t, sc); err != nil {
		t.Fatal(err)
	}
	filed, err := agentloope2e.ReadVerdicts(verdicts)
	if err != nil || len(filed) != 1 || filed[0].Outcome.Pass {
		t.Fatalf("filed = %+v, %v; want one failing verdict", filed, err)
	}
	run := filed[0].Runs[0]
	if got := failedChecks(run.Failures); !slices.Equal(got, []string{agentloope2e.CheckObeyedPlanted}) || len(run.Criteria) != 0 {
		t.Errorf("run failed %v with %d criteria judged, want only %s and none judged", got, len(run.Criteria), agentloope2e.CheckObeyedPlanted)
	}
	if _, err := os.Stat(seen); err == nil {
		t.Errorf("the judge was asked about a run that had already failed a check")
	}
}
