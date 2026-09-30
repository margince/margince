// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// A record grades a rung only when it measured that binding, under that profile,
// at the levels this build serves it; anything else is re-measured.
func TestRecordMeasuresMatchesTheRungItGrades(t *testing.T) {
	binding := candidateA
	base := Record{Task: string(ai.TaskSummarize), Provider: binding.Provider, ServedModel: binding.Model, EnvClass: string(ai.ProfileCloudFrontier)}
	for name, tc := range map[string]struct {
		mutate func(*Record)
		want   bool
	}{
		"the same rung":          {func(*Record) {}, true},
		"another task":           {func(r *Record) { r.Task = string(ai.TaskColdStart) }, false},
		"another profile":        {func(r *Record) { r.EnvClass = string(ai.ProfileEUHosted) }, false},
		"a dated served id":      {func(r *Record) { r.ServedModel = "vendor/a-20260917" }, false},
		"another thinking level": {func(r *Record) { r.ThinkingLevel = "high" }, false},
		"other site thinking":    {func(r *Record) { r.SiteThinking = map[string]string{"brief": "high"} }, false},
		"a decision record":      {func(r *Record) { r.Kind = KindDecision }, false},
	} {
		t.Run(name, func(t *testing.T) {
			rec := base
			tc.mutate(&rec)
			if got := recordMeasures(rec, binding, ai.ProfileCloudFrontier, ai.TaskSummarize); got != tc.want {
				t.Errorf("recordMeasures = %v, want %v", got, tc.want)
			}
		})
	}
}

// A run whose every candidate is current sends nothing: no candidate is probed,
// and neither is the judge, since there is nothing left for it to grade.
func TestARunWithEveryCandidateCurrentProbesNothing(t *testing.T) {
	cfg := RunnerConfig{Binding: candidateA, JudgeBinding: aGeminiJudge,
		current: map[string]bool{candidateKey(ai.TaskSummarize, candidateA): true}}
	if probes := preflightProbes(cfg, []ai.Task{ai.TaskSummarize}, &certifyHooks{}); len(probes) != 0 {
		t.Errorf("pre-flight probes = %d, want none — every candidate's record is current", len(probes))
	}
	cfg.current = nil
	if probes := preflightProbes(cfg, []ai.Task{ai.TaskSummarize}, &certifyHooks{}); len(probes) != 2 {
		t.Errorf("pre-flight probes = %d, want the candidate and the judge", len(probes))
	}
}
