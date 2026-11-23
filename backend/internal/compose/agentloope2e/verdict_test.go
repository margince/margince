// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentloope2e

import (
	"os"
	"path/filepath"
	"reflect"
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
		out[i] = Run{Criteria: no()}
		if said {
			out[i].Criteria = yes()
		}
	}
	return out
}

func TestDecideAppliesThePassBar(t *testing.T) {
	failedCheck := append(runsOf(true, true), Run{Failures: []Failure{{Check: "forbidden", Detail: "d"}}, Criteria: yes()})
	unjudged := []Run{{}, {}, {}}
	for name, tc := range map[string]struct {
		runs []Run
		pass bool
	}{
		"three clean runs, the judge says yes to all":     {runsOf(true, true, true), true},
		"two of three the judge passes":                   {runsOf(true, false, true), true},
		"only one of three the judge passes":              {runsOf(true, false, false), false},
		"one run failed a check, whatever the judge said": {failedCheck, false},
		"a judge that was never asked passes no run":      {unjudged, false},
		"no runs":                   {nil, false},
		"one run the judge refuses": {runsOf(false), false},
		"one run the judge passes":  {runsOf(true), true},
		"two of four is short":      {runsOf(true, true, false, false), false},
		"three of four is enough":   {runsOf(true, true, true, false), true},
		"seven of twelve is short":  {runsOf(slices.Concat(slices.Repeat([]bool{true}, 7), slices.Repeat([]bool{false}, 5))...), false},
		"eight of twelve is enough": {runsOf(slices.Concat(slices.Repeat([]bool{true}, 8), slices.Repeat([]bool{false}, 4))...), true},
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
	run := Run{Criteria: append(yes(), Criterion{Text: "no invented figure", Met: false, Reason: "it quotes one"})}
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
	binding := Binding{Provider: "gemini", Model: "gemini-3.8-flash", Tier: "premium"}
	was := NewStamp("a", "b", "c", binding, "judge-1")
	if got := was.Moved(was); len(got) != 0 {
		t.Errorf("an unchanged stamp moved %v", got)
	}
	other := Binding{Provider: "gemini", Model: "gemini-3.8-flash", Tier: "cheap"}
	now := NewStamp("a", "B", "c", other, "judge-2")
	if got := was.Moved(now); !slices.Equal(got, []string{"agent window", "binding", "judge"}) {
		t.Errorf("moved = %v, want the window, the binding and the judge", got)
	}
}

func fullVerdict() Verdict {
	binding := Binding{Provider: "gemini", Model: "gemini-3.8-flash", Tier: "premium"}
	runs := []Run{
		{Criteria: yes()},
		{Failures: []Failure{{Check: "forbidden", Detail: "wrote on deal_x"}}, Criteria: no()},
		{Criteria: yes()},
	}
	return Verdict{
		Scenario: "annotates", Agent: MorningBrief, Binding: binding, Judge: "claude-haiku-5-5", SelfJudged: false,
		Runs: runs, Outcome: Decide(runs), Stamp: NewStamp("s", "w", "h", binding, "claude-haiku-5-5"), RanAt: "2026-11-23T10:00:00Z",
	}
}

func TestAVerdictRoundTripsWhole(t *testing.T) {
	root := t.TempDir()
	v := fullVerdict()
	if err := v.Write(root); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "gemini@gemini-3.8-flash@premium", "annotates.json"); v.Path(root) != want {
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
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], v) {
		t.Errorf("read back %+v, %v; want the verdict written", got, err)
	}
}

// Two bindings that differ in one part, or whose parts only look alike once a
// separator moves, keep their own verdicts.
func TestBindingsThatDifferInAnyPartKeepTheirOwnVerdicts(t *testing.T) {
	root := t.TempDir()
	for _, binding := range []Binding{
		{Provider: "gemini", Model: "a/b", Tier: "premium"},
		{Provider: "gemini", Model: "a_b", Tier: "premium"},
		{Provider: "vertex", Model: "a/b", Tier: "premium"},
		{Provider: "gemini", Model: "a/b", Tier: "cheap"},
		{Provider: "p", Model: "x@y", Tier: "z"},
		{Provider: "p", Model: "x", Tier: "y@z"},
	} {
		if err := (Verdict{Scenario: "s", Binding: binding}).Write(root); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ReadVerdicts(root)
	if err != nil || len(got) != 6 {
		t.Errorf("read %d verdicts, %v; want 6, none overwritten", len(got), err)
	}
}

func TestAVerdictThatCannotBeFiledIsRefused(t *testing.T) {
	good := fullVerdict()
	for name, edit := range map[string]func(*Verdict){
		"a name that leaves the folder": func(v *Verdict) { v.Scenario = "../x" },
		"a name in upper case":          func(v *Verdict) { v.Scenario = "Annotates" },
		"no provider":                   func(v *Verdict) { v.Binding.Provider = "" },
		"no model":                      func(v *Verdict) { v.Binding.Model = "" },
		"no tier":                       func(v *Verdict) { v.Binding.Tier = "" },
	} {
		t.Run(name, func(t *testing.T) {
			v := good
			edit(&v)
			if err := v.Write(t.TempDir()); err == nil {
				t.Error("a verdict that no reader could find was written")
			}
		})
	}
}

func TestAMissingRootHoldsNoVerdict(t *testing.T) {
	got, err := ReadVerdicts(filepath.Join(t.TempDir(), "none"))
	if err != nil || len(got) != 0 {
		t.Errorf("read %v, %v from a root that does not exist", got, err)
	}
}

func TestARootWithPatternCharactersIsReadAsItIs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "records [a]*")
	v := fullVerdict()
	if err := v.Write(root); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadVerdicts(root); err != nil || len(got) != 1 {
		t.Errorf("read %d verdicts, %v, from a root with glob characters", len(got), err)
	}
}

func TestAVerdictOutOfPlaceIsRefused(t *testing.T) {
	root := t.TempDir()
	v := fullVerdict()
	if err := v.Write(root); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "other@m@t")
	if err := os.MkdirAll(moved, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(v.Path(root), filepath.Join(moved, "annotates.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadVerdicts(root); err == nil || !strings.Contains(err.Error(), "wrong place") {
		t.Errorf("err = %v, want a verdict moved into another binding's folder refused", err)
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
