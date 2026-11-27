// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package live

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
)

// repoRoot walks up to the directory holding go.work, the repository root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.work above the test's folder")
		}
		dir = parent
	}
}

// labRoot copies the files a stamp reads into a temporary root, so a test can
// change one and watch the stamp. It copies what the stamp's own globs match.
func labRoot(t *testing.T) string {
	t.Helper()
	source, lab := repoRoot(t), t.TempDir()
	patterns := []string{
		"e2e/llm/candidates.json", "e2e/llm/candidates.py", "e2e/llm/judge.py", "e2e/llm/judge_cli.py", DefaultPreset,
		runnerDir + "/window.go", HarnessDir + "/runner_integration_test.go",
		"backend/internal/compose/registry.go", "backend/internal/compose/fortest_integration.go",
		"backend/internal/compose/loopfault/*.go", "backend/internal/modules/agents/tools.go",
		"backend/internal/modules/agents/registry.go", "backend/internal/modules/agents/tools_readfailure.go",
		lanePackage + "/*.go", lanePackage + "/live/*.go",
		HarnessDir + "/agentloop_*_test.go", ScenarioDir + "/*.yaml",
	}
	for _, pattern := range patterns {
		found, err := filepath.Glob(filepath.Join(source, pattern))
		if err != nil || len(found) == 0 {
			t.Fatalf("%s matched nothing under the repository: %v", pattern, err)
		}
		for _, path := range found {
			rel, err := filepath.Rel(source, path)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(lab, rel), string(body))
		}
	}
	return lab
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func briefIn(t *testing.T, root string) agentloope2e.Scenario {
	t.Helper()
	all, err := agentloope2e.LoadScenarios(filepath.Join(root, ScenarioDir))
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(sc agentloope2e.Scenario) bool { return sc.Name == briefScenario })
	if i < 0 {
		t.Fatalf("no scenario %s", briefScenario)
	}
	return all[i]
}

const briefScenario = "morning_brief_annotates_its_queue_from_the_evidence"

var testBinding = agentloope2e.Binding{Provider: "gemini", Model: "gemini-3.8-flash", Tier: "premium"}

// Each part of the stamp must move when, and only when, its own input does.
// A part that never moves would call a verdict current after the thing it
// measured had changed.
func TestEachStampPartMovesWithItsOwnInput(t *testing.T) {
	root := labRoot(t)
	sc := briefIn(t, root)
	base, err := CurrentStamp(root, "", sc, testBinding)
	if err != nil {
		t.Fatal(err)
	}
	again, err := CurrentStamp(root, "", sc, testBinding)
	if err != nil || again != base {
		t.Fatalf("the stamp of an unchanged tree changed: %v\n%+v\n%+v", err, base, again)
	}
	t.Run("scenario", func(t *testing.T) {
		lab := labRoot(t)
		path := filepath.Join(lab, ScenarioDir, briefScenario+".yaml")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, path, strings.Replace(string(body), "by Wednesday", "by Tuesday", 1))
		assertMoved(t, base, stampOf(t, lab, "", testBinding), "scenario")
	})
	t.Run("harness", func(t *testing.T) {
		for name, edit := range map[string]func(lab string){
			"a harness test file": func(lab string) {
				appendTo(t, filepath.Join(lab, HarnessDir, "agentloop_models_integration_test.go"))
			},
			"fault reader":          func(lab string) { appendTo(t, filepath.Join(lab, "backend/internal/compose/loopfault/reader.go")) },
			"fault registry wiring": func(lab string) { appendTo(t, filepath.Join(lab, "backend/internal/compose/registry.go")) },
			"fault read boundary": func(lab string) {
				appendTo(t, filepath.Join(lab, "backend/internal/modules/agents/tools_readfailure.go"))
			},
			"a lane source file": func(lab string) { appendTo(t, filepath.Join(lab, lanePackage, "check.go")) },
			"the live package":   func(lab string) { appendTo(t, filepath.Join(lab, lanePackage, "live", "certify.go")) },
			"the judge":          func(lab string) { appendTo(t, filepath.Join(lab, "e2e/llm/judge.py")) },
			"the judge's door":   func(lab string) { appendTo(t, filepath.Join(lab, "e2e/llm/judge_cli.py")) },
			"the runner frame":   func(lab string) { appendTo(t, filepath.Join(lab, runnerDir, "window.go")) },
			"the runner wiring":  func(lab string) { appendTo(t, filepath.Join(lab, HarnessDir, "runner_integration_test.go")) },
			"a new harness file": func(lab string) {
				writeFile(t, filepath.Join(lab, HarnessDir, "agentloop_extra_integration_test.go"), "package jobfanout\n")
			},
		} {
			lab := labRoot(t)
			edit(lab)
			if moved := base.Moved(stampOf(t, lab, "", testBinding)); !slices.Equal(moved, []string{"harness"}) {
				t.Errorf("%s moved %v, want only the harness", name, moved)
			}
		}
	})
	t.Run("binding", func(t *testing.T) {
		other := agentloope2e.Binding{Provider: "gemini", Model: "gemini-3.1-flash-lite", Tier: "cheap_cloud"}
		assertMoved(t, base, stampOf(t, root, "", other), "binding")
	})
	t.Run("judge", func(t *testing.T) {
		assertMoved(t, base, stampOf(t, root, "claude-another-judge", testBinding), "judge")
	})
}

func appendTo(t *testing.T, path string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(body)+"\n// changed\n")
}

func stampOf(t *testing.T, root, judge string, binding agentloope2e.Binding) agentloope2e.Stamp {
	t.Helper()
	stamp, err := CurrentStamp(root, judge, briefIn(t, root), binding)
	if err != nil {
		t.Fatal(err)
	}
	return stamp
}

func assertMoved(t *testing.T, was, now agentloope2e.Stamp, parts ...string) {
	t.Helper()
	moved := was.Moved(now)
	if !slices.Equal(moved, parts) {
		t.Errorf("moved %v, want %v", moved, parts)
	}
}

// The agent's window moves with its goal, each tool's published spec and the
// list of tools. The two scheduled agents hold different windows.
func TestTheWindowMovesWithGoalToolsAndTheirSpecs(t *testing.T) {
	read := toolFacts{Name: "read_brief", Description: "Reads the brief.", ReadOnly: true}
	write := toolFacts{Name: "annotate_brief", Description: "Writes a finding.", Scope: "write"}
	digest := func(goal string, tools ...toolFacts) string {
		t.Helper()
		got, err := windowOf(goal, tools)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	base := digest("Annotate the queue.", read, write)
	if digest("Annotate the queue.", read, write) != base {
		t.Fatal("the same window digests two ways")
	}
	reworded := write
	reworded.Description = "Writes a finding on one item."
	widened := write
	widened.Scope = "admin"
	reschema := read
	reschema.InputSchema = []byte(`{"type":"object"}`)
	for name, other := range map[string]string{
		"the goal":           digest("Annotate the queue now.", read, write),
		"a tool's copy":      digest("Annotate the queue.", read, reworded),
		"a tool's scope":     digest("Annotate the queue.", read, widened),
		"a tool's schema":    digest("Annotate the queue.", reschema, write),
		"a tool added":       digest("Annotate the queue.", read, write, toolFacts{Name: "list_records"}),
		"a tool taken away":  digest("Annotate the queue.", read),
		"the order of tools": digest("Annotate the queue.", write, read),
	} {
		if other == base {
			t.Errorf("a change to %s left the window as it was", name)
		}
	}
	brief, err := windowDigest(agentloope2e.MorningBrief)
	if err != nil {
		t.Fatal(err)
	}
	sweep, err := windowDigest(agentloope2e.OvernightSweep)
	if err != nil || brief == sweep {
		t.Errorf("the two agents share a window: %q, %v", brief, err)
	}
	if _, err := windowDigest("no_such_agent"); err == nil {
		t.Error("a window for an agent the catalog does not schedule was digested")
	}
}

// The judge name a verdict holds is the one judge_cli.py asks. The pinned model
// has one reader, and a run that named another passes what that reader reports.
func TestTheStampNamesTheModelTheJudgeIsPinnedTo(t *testing.T) {
	root := repoRoot(t)
	pinned, err := JudgeName(root, "")
	if err != nil {
		t.Fatal(err)
	}
	door := agentloope2e.CommandJudge{Root: root, Env: []string{"PATH=" + os.Getenv("PATH")}}
	if asked, err := door.Model(context.Background()); err != nil || asked != pinned {
		t.Errorf("the stamp names %q, the judge is pinned to %q (%v)", pinned, asked, err)
	}
	named := agentloope2e.CommandJudge{Root: root, Env: append(door.Env, "E2E_LLM_JUDGE_MODEL=claude-another-judge")}
	asked, err := named.Model(context.Background())
	if err != nil || asked != "claude-another-judge" {
		t.Fatalf("the judge asks %q, %v when another model is named", asked, err)
	}
	if got, err := JudgeName(root, asked); err != nil || got != asked {
		t.Errorf("a named judge model is stamped as %q, %v", got, err)
	}
}

type stubJudge struct {
	asked   int
	answers map[string]bool
	err     error
}

func (j *stubJudge) Grade(_ context.Context, criteria []string, answer string) ([]agentloope2e.Criterion, error) {
	j.asked++
	if j.err != nil {
		return nil, j.err
	}
	var out []agentloope2e.Criterion
	for _, text := range criteria {
		met, known := j.answers[text]
		out = append(out, agentloope2e.Criterion{Text: text, Met: !known || met, Reason: "stub saw " + string(rune('0'+len(answer)%10))})
	}
	return out, nil
}

func goodRun(sc agentloope2e.Scenario) Observed {
	var writes []agentloope2e.Write
	for _, subject := range sc.Worklist {
		writes = append(writes, agentloope2e.Write{Tool: "annotate_brief", Subject: subject, Text: "a finding"})
	}
	return Observed{Snapshot: agentloope2e.Snapshot{RunStatus: runner.OutcomeCompleted, Writes: writes}, SeededAt: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)}
}

func certifyConfig(root, dir string, judge agentloope2e.Judge, runs int) Config {
	return Config{
		Root: root, VerdictDir: dir, Binding: testBinding, Judge: judge, Runs: runs,
		Now: func() time.Time { return time.Date(2026, 11, 24, 9, 30, 0, 0, time.UTC) },
	}
}

func TestCertifyFilesAVerdictTheReportReadsBackAsCurrent(t *testing.T) {
	root, dir := labRoot(t), filepath.Join(t.TempDir(), "verdicts")
	sc := briefIn(t, root)
	judge := &stubJudge{}
	verdict, err := Certify(context.Background(), certifyConfig(root, dir, judge, 3), sc, func(int) (Observed, error) { return goodRun(sc), nil })
	if err != nil {
		t.Fatal(err)
	}
	if !verdict.Outcome.Pass || !verdict.Outcome.Smoke || verdict.Outcome.Passed != 3 || judge.asked != 3 {
		t.Errorf("outcome = %+v after %d judge calls, want a passing smoke result of 3", verdict.Outcome, judge.asked)
	}
	if verdict.RanAt != "2026-11-24T09:30:00Z" || verdict.Judge == "" || verdict.SelfJudged {
		t.Errorf("verdict = %+v", verdict)
	}
	lines, err := Lines(root, dir, "")
	if err != nil || len(lines) != 1 || !lines[0].Current() || lines[0].Verdict.Stamp != verdict.Stamp {
		t.Fatalf("read back %+v, %v; want one current line", lines, err)
	}
	if current, err := IsCurrent(root, dir, "", sc, testBinding); err != nil || !current {
		t.Errorf("IsCurrent = %v, %v", current, err)
	}
	if current, err := IsCurrent(root, dir, "", sc, agentloope2e.Binding{Provider: "gemini", Model: "gemini-3.1-flash-lite", Tier: "cheap_cloud"}); err != nil || current {
		t.Error("a verdict on another binding counted as current")
	}
}

func TestAVerdictGoesStaleNamingThePartThatMoved(t *testing.T) {
	root, dir := labRoot(t), filepath.Join(t.TempDir(), "verdicts")
	sc := briefIn(t, root)
	if _, err := Certify(context.Background(), certifyConfig(root, dir, &stubJudge{}, 1), sc, func(int) (Observed, error) { return goodRun(sc), nil }); err != nil {
		t.Fatal(err)
	}
	appendTo(t, filepath.Join(root, "e2e/llm/judge.py"))
	lines, err := Lines(root, dir, "")
	if err != nil || len(lines) != 1 || !slices.Equal(lines[0].Moved, []string{"harness"}) {
		t.Fatalf("lines = %+v, %v; want one stale on the harness", lines, err)
	}
	if current, err := IsCurrent(root, dir, "", sc, testBinding); err != nil || current {
		t.Error("a stale verdict counted as current")
	}
	if err := os.Remove(filepath.Join(root, ScenarioDir, briefScenario+".yaml")); err != nil {
		t.Fatal(err)
	}
	if lines, err = Lines(root, dir, ""); err != nil || !slices.Equal(lines[0].Moved, []string{"scenario removed"}) {
		t.Errorf("a verdict for a removed scenario = %+v, %v", lines, err)
	}
}

func TestAVerdictGoesStaleWhenThePresetBindsAnotherModelAtItsTier(t *testing.T) {
	root, dir := labRoot(t), filepath.Join(t.TempDir(), "verdicts")
	sc := briefIn(t, root)
	if _, err := Certify(context.Background(), certifyConfig(root, dir, &stubJudge{}, 1), sc, func(int) (Observed, error) { return goodRun(sc), nil }); err != nil {
		t.Fatal(err)
	}
	preset := filepath.Join(root, DefaultPreset)
	body, err := os.ReadFile(preset)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, preset, strings.ReplaceAll(string(body), testBinding.Model, "gemini-9-flash"))
	lines, err := Lines(root, dir, "")
	if err != nil || len(lines) != 1 || !slices.Equal(lines[0].Moved, []string{"harness", "binding"}) {
		t.Fatalf("lines = %+v, %v; want one stale on the harness and the binding", lines, err)
	}
}

func TestACheckFailureIsNotJudgedAndTheBarCountsItOut(t *testing.T) {
	root, dir := labRoot(t), filepath.Join(t.TempDir(), "verdicts")
	sc := briefIn(t, root)
	judge := &stubJudge{}
	bad := Observed{Snapshot: agentloope2e.Snapshot{RunStatus: runner.OutcomeCompleted}}
	verdict, err := Certify(context.Background(), certifyConfig(root, dir, judge, 2), sc, func(run int) (Observed, error) {
		if run == 1 {
			return goodRun(sc), nil
		}
		return bad, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Outcome.Pass || judge.asked != 1 || len(verdict.Runs[1].Criteria) != 0 || verdict.Runs[1].ChecksPassed() {
		t.Errorf("outcome %+v, %d judge calls, run 2 = %+v; want a failure with the failed run never judged", verdict.Outcome, judge.asked, verdict.Runs[1])
	}
}

func TestAHarnessFailureStopsThePassAndFilesNothing(t *testing.T) {
	root := labRoot(t)
	sc := briefIn(t, root)
	boom := errors.New("the seed probe failed")
	for name, tc := range map[string]struct {
		judge   *stubJudge
		attempt func(int) (Observed, error)
		calls   *int
	}{
		"an attempt that fails": {judge: &stubJudge{}, attempt: func(int) (Observed, error) { return Observed{}, boom }},
		"a judge that is unavailable": {
			judge:   &stubJudge{err: agentloope2e.ErrJudgeUnavailable},
			attempt: func(int) (Observed, error) { return goodRun(sc), nil },
		},
	} {
		dir, runs := filepath.Join(t.TempDir(), "verdicts"), 0
		attempt := func(run int) (Observed, error) { runs++; return tc.attempt(run) }
		_, err := Certify(context.Background(), certifyConfig(root, dir, tc.judge, 3), sc, attempt)
		if err == nil || runs != 1 {
			t.Errorf("%s: err = %v after %d attempt(s), want an error after the first", name, err, runs)
		}
		if _, statErr := os.Stat(dir); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("%s: a verdict was filed (%v)", name, statErr)
		}
	}
	if _, err := Certify(context.Background(), certifyConfig(root, t.TempDir(), &stubJudge{}, 0), sc, nil); err == nil {
		t.Error("a pass of no runs was certified")
	}
}

func TestACandidateJudgedByItsOwnFamilyIsSelfJudged(t *testing.T) {
	root, dir := labRoot(t), filepath.Join(t.TempDir(), "verdicts")
	sc := briefIn(t, root)
	cfg := certifyConfig(root, dir, &stubJudge{}, 1)
	cfg.JudgeModel = "gemini-3.1-flash-lite"
	verdict, err := Certify(context.Background(), cfg, sc, func(int) (Observed, error) { return goodRun(sc), nil })
	if err != nil || !verdict.SelfJudged {
		t.Errorf("verdict = %+v, %v; want self_judged for a Gemini model judged by a Gemini model", verdict.SelfJudged, err)
	}
}
