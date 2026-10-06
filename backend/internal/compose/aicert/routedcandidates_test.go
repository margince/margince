// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

var (
	candidateA   = ai.ProviderConfig{Provider: "openai_compatible", Model: "vendor/a"}
	candidateB   = ai.ProviderConfig{Provider: "openai_compatible", Model: "vendor/b"}
	aGeminiJudge = ai.ProviderConfig{Provider: "gemini", Model: "gemini-3.5-flash"}
)

func routedTo(tiers map[ai.Tier]ai.ProviderConfig) RunnerConfig {
	return RunnerConfig{Routing: &ai.RoutingConfig{Profile: ai.ProfileCloudFrontier, Tiers: tiers}, JudgeBinding: aGeminiJudge}
}

func candidateModels(cands []candidate) []string {
	var models []string
	for _, c := range cands {
		models = append(models, c.Binding.Model)
	}
	return models
}

// A routed run certifies each model the task's ladder binds, in the order the
// router walks them, and a rung binding a model above it adds no candidate.
func TestARoutedTaskIsCertifiedOnEveryDistinctRung(t *testing.T) {
	ladder := ai.TaskLadder(ai.TaskSummarize)
	if len(ladder) != 2 {
		t.Fatalf("%s's ladder is %v; this test needs two rungs", ai.TaskSummarize, ladder)
	}
	for name, tc := range map[string]struct {
		tiers map[ai.Tier]ai.ProviderConfig
		want  []string
	}{
		"two models":         {map[ai.Tier]ai.ProviderConfig{ladder[0]: candidateA, ladder[1]: candidateB}, []string{"vendor/a", "vendor/b"}},
		"same model twice":   {map[ai.Tier]ai.ProviderConfig{ladder[0]: candidateA, ladder[1]: candidateA}, []string{"vendor/a"}},
		"lower rung unbound": {map[ai.Tier]ai.ProviderConfig{ladder[0]: candidateA}, []string{"vendor/a"}},
		"first rung unbound": {map[ai.Tier]ai.ProviderConfig{ladder[1]: candidateB}, []string{"vendor/b"}},
	} {
		t.Run(name, func(t *testing.T) {
			cands, skipped, err := taskCandidates(routedTo(tc.tiers), ai.TaskSummarize)
			if err != nil {
				t.Fatal(err)
			}
			if got := candidateModels(cands); !slices.Equal(got, tc.want) {
				t.Errorf("candidates = %v, want %v", got, tc.want)
			}
			if len(skipped) != 0 {
				t.Errorf("skipped %+v, want none", skipped)
			}
			if cands[0].Rung != 0 {
				t.Errorf("the first candidate is rung %d, want 0 — it is the model that answers", cands[0].Rung)
			}
		})
	}
}

// A MODEL= run names one candidate outright and has no ladder to walk.
func TestAModelRunHasOneCandidate(t *testing.T) {
	cands, skipped, err := taskCandidates(RunnerConfig{Binding: candidateA, JudgeBinding: aGeminiJudge}, ai.TaskSummarize)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || !sameModel(cands[0].Binding, candidateA) || cands[0].Rung != 0 || cands[0].Tier != "" || len(skipped) != 0 {
		t.Errorf("candidates = %+v, skipped = %+v, want vendor/a alone at rung 0 with no tier", cands, skipped)
	}
}

// A fallback the judge cannot grade is left unmeasured and named, rather than
// costing the task the first rung's record.
func TestAFallbackInTheJudgesFamilyIsSkippedNotRefused(t *testing.T) {
	ladder := ai.TaskLadder(ai.TaskSummarize)
	cands, skipped, err := taskCandidates(routedTo(map[ai.Tier]ai.ProviderConfig{ladder[0]: candidateA, ladder[1]: aGeminiJudge}), ai.TaskSummarize)
	if err != nil {
		t.Fatalf("a fallback in the judge's family refused the task: %v", err)
	}
	if got := candidateModels(cands); !slices.Equal(got, []string{"vendor/a"}) {
		t.Errorf("candidates = %v, want vendor/a alone", got)
	}
	if len(skipped) != 1 || skipped[0].Model != aGeminiJudge.Model || skipped[0].Reason != "the judge's own family" {
		t.Errorf("skipped = %+v, want the judge's model named with its reason", skipped)
	}
}

// The rung that answers is still refused before any paid call when it is the
// judge: a model grading itself is certified by construction.
func TestAFirstRungInTheJudgesFamilyStillRefusesTheRun(t *testing.T) {
	ladder := ai.TaskLadder(ai.TaskSummarize)
	cfg := routedTo(map[ai.Tier]ai.ProviderConfig{ladder[0]: aGeminiJudge, ladder[1]: candidateA})
	if _, _, err := taskCandidates(cfg, ai.TaskSummarize); !errors.Is(err, errSelfJudged) {
		t.Errorf("taskCandidates err = %v, want errSelfJudged", err)
	}
	if err := refuseSelfJudgedTasks(cfg, []ai.Task{ai.TaskSummarize}); !errors.Is(err, errSelfJudged) {
		t.Errorf("refuseSelfJudgedTasks err = %v, want errSelfJudged before any call", err)
	}
}
