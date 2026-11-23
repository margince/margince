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

func yes() []Criterion {
	return []Criterion{{Text: "names the evidence", Met: true, Reason: "it does"}}
}

func no() []Criterion {
	return []Criterion{{Text: "names the evidence", Met: false, Reason: "it does not"}}
}

// runsOf builds one run per answer: true for a run whose judge said yes.
func runsOf(judged ...bool) []Run {
	out := make([]Run, len(judged))
	for i, said := range judged {
		out[i] = Run{ChecksPassed: true, Criteria: no()}
		if said {
			out[i].Criteria = yes()
		}
	}
	return out
}

func TestDecideAppliesThePassBar(t *testing.T) {
	failedCheck := append(runsOf(true, true), Run{ChecksPassed: false, Criteria: yes()})
	unjudged := []Run{{ChecksPassed: true}, {ChecksPassed: true}, {ChecksPassed: true}}
	for name, tc := range map[string]struct {
		runs []Run
		pass bool
	}{
		"three clean runs, the judge says yes to all":     {runsOf(true, true, true), true},
		"two of three the judge passes":                   {runsOf(true, false, true), true},
		"only one of three the judge passes":              {runsOf(true, false, false), false},
		"one run failed a check, whatever the judge said": {failedCheck, false},
		"a judge that was never asked passes no run":      {unjudged, false},
		"no runs": {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := Decide(tc.runs)
			if got.Pass != tc.pass || got.Runs != len(tc.runs) {
				t.Errorf("Decide = %+v, want pass %v over %d runs", got, tc.pass, len(tc.runs))
			}
		})
	}
}

func TestOneNoAmongManyYesesFailsTheRunsJudgement(t *testing.T) {
	run := Run{ChecksPassed: true, Criteria: append(yes(), Criterion{Text: "no invented figure", Met: false, Reason: "it quotes one"})}
	if run.JudgePassed() {
		t.Error("a run with one criterion unmet passed the judge")
	}
}

func TestFewRunsAreASmokeResult(t *testing.T) {
	if !Decide(runsOf(true, true, true)).Smoke {
		t.Error("three runs were not called a smoke result")
	}
	if Decide(runsOf(slices.Repeat([]bool{true}, smokeRuns)...)).Smoke {
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
		Judge: "claude-haiku-5-5", Runs: runsOf(true, true, true), Outcome: Decide(runsOf(true, true, true)),
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
