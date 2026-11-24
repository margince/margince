// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && e2e_agent_loop

package jobfanout

// The paid agent_loop lane: the scheduled agents run on a seeded workspace
// against a real model route, and a judge reads what they wrote. It costs real
// tokens, so it sits behind its own build tag and an opt-in variable, and it
// runs only through `make e2e-agent-loop`. Its name does not match the scripted
// lane's run pattern, so the free lane never reaches it.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/agentloope2e/live"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/clockskew"
)

// The settings `make e2e-agent-loop` passes. Each is read here and nowhere else.
const (
	envOptIn     = "E2E_AGENT_LOOP"
	envRuns      = "E2E_AGENT_LOOP_RUNS"
	envScenario  = "E2E_AGENT_LOOP_SCENARIO"
	envTier      = "E2E_AGENT_LOOP_TIER"
	envModel     = "E2E_AGENT_LOOP_MODEL"
	envBaseURL   = "E2E_AGENT_LOOP_BASE_URL"
	envPreset    = "E2E_AGENT_LOOP_PRESET"
	envStaleOnly = "E2E_AGENT_LOOP_STALE_ONLY"
	envTrace     = "E2E_AGENT_LOOP_TRACE"

	defaultRuns   = 3
	defaultPreset = "config/presets/gemini_cloud.yaml"
)

func TestE2EAgentLoopLive(t *testing.T) {
	if os.Getenv(envOptIn) == "" {
		t.Fatalf("TestE2EAgentLoopLive requires %s=1 (set by `make e2e-agent-loop`): this lane costs real tokens, so it never runs implicitly", envOptIn)
	}
	if os.Getenv(clockskew.EnvVar) != "" {
		t.Skip("the lane reads day-level facts off the real clock, which the drift lane moves")
	}
	for _, name := range []string{"MARGINCE_TEST_DSN", "MARGINCE_TEST_APP_DSN"} {
		if err := live.RequireLocalDSN(os.Getenv(name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	root := integration.RepositoryRoot(t)
	lane, scenarios := paidLane(t, root)
	for _, sc := range scenarios {
		if current, err := live.IsCurrent(root, lane.cfg.VerdictDir, lane.cfg.JudgeModel, sc, lane.cfg.Binding); err != nil {
			t.Fatal(err)
		} else if current && os.Getenv(envStaleOnly) != "0" {
			t.Logf("%s: the verdict on %s is current; STALE_ONLY=0 measures it again", sc.Name, lane.cfg.Binding.Key())
			continue
		}
		// A failed case is a harness failure, since a model that failed its
		// checks is a verdict and not a failure. It stops the pass before the next
		// case pays for more calls.
		if !t.Run(sc.Name, func(t *testing.T) { reportVerdict(t, lane, sc) }) {
			t.Fatalf("stopping at %s, the first harness failure, so no more calls are paid for", sc.Name)
		}
	}
}

func reportVerdict(t *testing.T, lane liveLane, sc agentloope2e.Scenario) {
	t.Helper()
	verdict, err := lane.certify(t, sc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s on %s: pass=%v smoke=%v, %d of %d runs passed the checks and the judge",
		sc.Name, verdict.Binding.Key(), verdict.Outcome.Pass, verdict.Outcome.Smoke, verdict.Outcome.Passed, verdict.Outcome.Runs)
}

// paidLane reads the settings, resolves the model route and asks the judge
// whether it can answer, all before the first model call is paid for.
func paidLane(t *testing.T, root string) (liveLane, []agentloope2e.Scenario) {
	t.Helper()
	runs := defaultRuns
	if raw := os.Getenv(envRuns); raw != "" {
		var err error
		if runs, err = strconv.Atoi(raw); err != nil || runs < 1 {
			t.Fatalf("%s=%q is not a positive number of runs", envRuns, raw)
		}
	}
	preset := os.Getenv(envPreset)
	if preset == "" {
		preset = filepath.Join(root, defaultPreset)
	}
	choice, err := live.ResolveChoice(preset, os.Getenv(envTier), os.Getenv(envModel), os.Getenv(envBaseURL))
	if err != nil {
		t.Fatalf("harness: the model route is not configured: %v", err)
	}
	if _, err := choice.ModelPath(&live.Recorder{}); err != nil {
		t.Fatalf("harness: the model route is not configured: %v", err)
	}
	judge := agentloope2e.CommandJudge{Root: root}
	if err := judge.Ready(context.Background()); err != nil {
		t.Fatalf("harness: %v", err)
	}
	judgeModel, err := judge.Model(context.Background())
	if err != nil {
		t.Fatalf("harness: %v", err)
	}
	scenarios := scenariosMatching(t, root, os.Getenv(envScenario))
	t.Logf("certifying %d scenario(s), %d run(s) each, on %s", len(scenarios), runs, choice.Binding.Key())
	lane := liveLane{
		cfg: live.Config{
			Root: root, VerdictDir: filepath.Join(root, live.VerdictDir), Binding: choice.Binding, Judge: judge, JudgeModel: judgeModel, Runs: runs,
		},
		traceDir: os.Getenv(envTrace),
		setup: func(t *testing.T) (*runnerEnv, *live.Recorder) {
			recorder := &live.Recorder{}
			path, err := choice.ModelPath(recorder)
			if err != nil {
				t.Fatalf("harness: the model route is not configured: %v", err)
			}
			return setupRunnerOn(t, &path), recorder
		},
		serve: func(_ *testing.T, re *runnerEnv, _ loopRun) *compose.RunnerService { return re.live },
	}
	return lane, scenarios
}

func scenariosMatching(t *testing.T, root, pattern string) []agentloope2e.Scenario {
	t.Helper()
	all, err := agentloope2e.LoadScenarios(filepath.Join(root, live.ScenarioDir))
	if err != nil {
		t.Fatal(err)
	}
	want, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("%s=%q: %v", envScenario, pattern, err)
	}
	var picked []agentloope2e.Scenario
	for _, sc := range all {
		if want.MatchString(sc.Name) {
			picked = append(picked, sc)
		}
	}
	if len(picked) == 0 {
		t.Fatalf("%s=%q matches none of the %d scenarios", envScenario, pattern, len(all))
	}
	return picked
}
