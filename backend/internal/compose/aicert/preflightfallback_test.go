// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// twoRungSummarize routes summarize to first, falling back to fallback.
func twoRungSummarize(first, fallback ai.ProviderConfig) RunnerConfig {
	ladder := ai.TaskLadder(ai.TaskSummarize)
	return RunnerConfig{
		Routing:      &ai.RoutingConfig{Profile: ai.ProfileCloudFrontier, Tiers: map[ai.Tier]ai.ProviderConfig{ladder[0]: first, ladder[1]: fallback}},
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
	}
}

// A fallback its host cannot serve costs the fallback's own record, never the
// run: the answering rung is still certified, and the fallback is named.
func TestAFallbackThePreflightCannotServeIsSkippedNotFatal(t *testing.T) {
	first := ai.ProviderConfig{Provider: ai.ProviderFake, Model: "answers"}
	fallback := ai.ProviderConfig{Provider: ai.ProviderFake, Model: "gone"}
	cfg := twoRungSummarize(first, fallback)
	candidates := ai.NewFakeClient().ScriptSteps(append([]ai.FakeStep{{Text: "ready"}}, failedWalk(t, errors.New("no endpoints found"))...)...)
	hooks := &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidates)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(ai.NewFakeClient().Script("ready"))},
	}
	unservable, err := preflight(wsContext(t), cfg, []ai.Task{ai.TaskSummarize}, hooks, quietLogger())
	if err != nil {
		t.Fatalf("a fallback's failed probe stopped the run: %v", err)
	}
	if len(unservable) != 1 || !sameModel(unservable[0], fallback) {
		t.Fatalf("unservable = %+v, want the fallback alone", unservable)
	}
	cfg.unservable = map[string]bool{bindingKey(fallback): true}
	cands, skipped, err := taskCandidates(cfg, ai.TaskSummarize)
	if err != nil || len(cands) != 1 || !sameModel(cands[0].Binding, first) {
		t.Fatalf("candidates = %+v (%v), want the answering rung alone", cands, err)
	}
	if len(skipped) != 1 || skipped[0].Model != fallback.Model {
		t.Errorf("skipped = %+v, want the unservable fallback named", skipped)
	}
}

// The model that answers failing its probe still stops the run before any
// scenario is paid for: without it there is nothing to certify.
func TestAnAnsweringRungThePreflightCannotServeStopsTheRun(t *testing.T) {
	cfg := twoRungSummarize(ai.ProviderConfig{Provider: ai.ProviderFake, Model: "answers"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "falls-back"})
	candidates := ai.NewFakeClient().ScriptSteps(append(failedWalk(t, errors.New("no endpoints found")), ai.FakeStep{Text: "ready"})...)
	hooks := &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidates)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(ai.NewFakeClient().Script("ready"))},
	}
	if _, err := preflight(wsContext(t), cfg, []ai.Task{ai.TaskSummarize}, hooks, quietLogger()); err == nil {
		t.Fatal("the answering rung failed its probe and the run went on")
	}
}
