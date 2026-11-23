// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentloope2e

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func passing(scores ...int) []Run {
	out := make([]Run, len(scores))
	for i, score := range scores {
		out[i] = Run{ChecksPassed: true, JudgeScore: score}
	}
	return out
}

func TestDecideAppliesThePassBar(t *testing.T) {
	for name, tc := range map[string]struct {
		runs []Run
		pass bool
	}{
		"three clean runs, all at the bar":           {passing(80, 75, 90), true},
		"two of three at the bar":                    {passing(80, 40, 90), true},
		"only one of three at the bar":               {passing(80, 40, 30), false},
		"one run failed a check, whatever it scored": {append(passing(80, 90), Run{ChecksPassed: false, JudgeScore: 100}), false},
		"no runs": {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := Decide(tc.runs, 70)
			if got.Pass != tc.pass || got.Runs != len(tc.runs) {
				t.Errorf("Decide = %+v, want pass %v over %d runs", got, tc.pass, len(tc.runs))
			}
		})
	}
}

func TestFewRunsAreASmokeResult(t *testing.T) {
	if !Decide(passing(90, 90, 90), 70).Smoke {
		t.Error("three runs were not called a smoke result")
	}
	if Decide(passing(slices.Repeat([]int{90}, smokeRuns)...), 70).Smoke {
		t.Errorf("%d runs were still called a smoke result", smokeRuns)
	}
}

func TestAStampNamesThePartThatMoved(t *testing.T) {
	was := Stamp{Scenario: "a", Window: "b", Harness: "c", Binding: "d", Judge: "e"}
	if got := was.Moved(was); len(got) != 0 {
		t.Errorf("an unchanged stamp moved %v", got)
	}
	now := Stamp{Scenario: "a", Window: "B", Harness: "c", Binding: "D", Judge: "e"}
	if got := was.Moved(now); !slices.Equal(got, []string{"agent window", "binding"}) {
		t.Errorf("moved = %v, want the window and the binding", got)
	}
}

func TestAVerdictRoundTripsUnderItsBindingFolder(t *testing.T) {
	root := t.TempDir()
	v := Verdict{
		Scenario: "annotates", Agent: MorningBrief, Binding: Binding{Provider: "gemini", Model: "gemini-3.8-flash", Tier: "premium"},
		Judge: "claude-sonnet-5-5", Runs: passing(80, 90, 85), Outcome: Decide(passing(80, 90, 85), 70),
		Stamp: Stamp{Scenario: "s"}, RanAt: "2026-11-23T10:00:00Z",
	}
	if err := v.Write(root); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "gemini-3.8-flash@premium", "annotates.json"); v.Path(root) != want {
		t.Errorf("path = %s, want %s", v.Path(root), want)
	}
	raw, err := os.ReadFile(v.Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"task"`) {
		t.Error("a verdict carries a task key, which the certification records would read as theirs")
	}
	got, err := ReadVerdicts(root)
	if err != nil || len(got) != 1 || got[0].Scenario != "annotates" || !got[0].Outcome.Pass || got[0].Stamp.Scenario != "s" {
		t.Errorf("read back %+v, %v", got, err)
	}
}

func TestAMissingRootHoldsNoVerdict(t *testing.T) {
	got, err := ReadVerdicts(filepath.Join(t.TempDir(), "none"))
	if err != nil || len(got) != 0 {
		t.Errorf("read %v, %v from a root that does not exist", got, err)
	}
}

func TestAnUnreadableVerdictIsRefusedByName(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "m@t"), 0o750); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(root, "m@t", "broken.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadVerdicts(root); err == nil || !strings.Contains(err.Error(), "broken.json") {
		t.Errorf("err = %v, want one naming the broken file", err)
	}
}
