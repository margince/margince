// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAVerdictReadsHowDirectlyItsRunsFinished(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "model-a")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"measured.json": `{"scenario": "case1_log", "passed": 2, "runs": 2, "pass_at": 2, "slow": true,
			"efficiency": {"runs": 2, "calls": {"median": 3.5, "min": 3, "max": 4},
			"turns": {"median": 5, "min": 5, "max": 5}, "refused": {"median": 0, "min": 0, "max": 0}}}`,
		"older.json": `{"scenario": "case2_read", "passed": 1, "runs": 1, "pass_at": 1}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	verdicts, err := ReadTree(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	cells := map[string][3]any{}
	for _, v := range verdicts {
		cells[v.Scenario] = [3]any{v.Efficiency.CallsCell(), v.Efficiency.TurnsCell(), v.Slow}
	}
	want := map[string][3]any{
		"case1_log":  {"3.5 (3–4)", "5", true},
		"case2_read": {Dash, Dash, false},
	}
	for scenario, cell := range want {
		if cells[scenario] != cell {
			t.Errorf("%s reads as %v, want %v", scenario, cells[scenario], cell)
		}
	}
}

func TestAModelsCallsPerCaseIsTheMedianOverTheCasesThatMeasuredThem(t *testing.T) {
	calls := func(median float64) *Efficiency {
		return &Efficiency{Runs: 1, Calls: &Spread{Median: median, Min: int(median), Max: int(median)}}
	}
	for _, tc := range []struct {
		measured []*Efficiency
		want     string
	}{
		{nil, Dash},
		{[]*Efficiency{nil, {Runs: 1}}, Dash},
		{[]*Efficiency{calls(9), nil, calls(2), calls(4)}, "4 over 3"},
		{[]*Efficiency{calls(2), calls(5)}, "3.5 over 2"},
	} {
		if got := CallsPerCase(tc.measured); got != tc.want {
			t.Errorf("CallsPerCase over %d verdicts = %q, want %q", len(tc.measured), got, tc.want)
		}
	}
}

func TestTheSummaryMediansShowADashForAMetricNoCaseCarried(t *testing.T) {
	measured := []*Efficiency{
		{Runs: 2, Calls: &Spread{Median: 4, Min: 3, Max: 5}},
		{Runs: 2, Calls: &Spread{Median: 7, Min: 7, Max: 7}},
	}
	if got := MedianCalls(measured); got != "5.5" {
		t.Errorf("MedianCalls = %q, want 5.5", got)
	}
	if got := MedianTurns(measured); got != Dash {
		t.Errorf("MedianTurns with no case counting turns = %q, want %q and never 0", got, Dash)
	}
	measured[0].Turns = &Spread{Median: 6, Min: 6, Max: 6}
	if got := MedianTurns(measured); got != "6" {
		t.Errorf("MedianTurns over the one case that counted them = %q, want 6", got)
	}
}
