// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// judgelessScenario is a loadable judge-less scenario with expect's tail
// replaced by tail.
func judgelessScenario(tail string) string {
	return `
name: x
task: summarize
site: widget
source: hand_authored
sanitized_by: jane
fixture: {subject: hi}
expect:
  outcome: accepted
` + tail
}

// A judge-less case loads with a reason and an answer, and nothing a judge
// would read; every other shape of it is a case graded by something nobody runs.
func TestLoadCorpusHoldsAJudgelessCaseToWhatMakesItHonest(t *testing.T) {
	for _, tc := range []struct {
		name, tail, wantErr string
	}{
		{name: "a reason and an answer load", tail: "  answer: hi\n  judge: none\n  judge_none_reason: the check reads the whole answer\n"},
		{name: "no reason", tail: "  answer: hi\n  judge: none\n", wantErr: "needs judge_none_reason"},
		{
			name:    "a rubric nobody reads",
			tail:    "  answer: hi\n  rubric: Score it.\n  judge: none\n  judge_none_reason: because\n",
			wantErr: "a rubric nobody reads",
		},
		{
			name:    "bands no score is held to",
			tail:    "  answer: hi\n  judge: none\n  judge_none_reason: because\n  bands: {certified_min: 70, degraded_min: 50, floor: 40}\n",
			wantErr: "quality bands",
		},
		{name: "no answer to hold the reply to", tail: "  judge: none\n  judge_none_reason: because\n", wantErr: "no expect.answer"},
		{name: "a judge named other than none", tail: "  answer: hi\n  judge: haiku\n", wantErr: `expect.judge is "haiku"`},
		{
			name:    "a reason on a judged case",
			tail:    "  answer: hi\n  judge_none_reason: because\n  bands: {certified_min: 70, degraded_min: 50, floor: 40}\n",
			wantErr: "set on a judged case",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeCorpusFile(t, dir, "summarize/one.yaml", judgelessScenario(tc.tail))
			loaded, err := aicert.LoadCorpus(dir, censusFor(t, ai.TaskSummarize))
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("LoadCorpus: %v", err)
			case tc.wantErr == "" && loaded[0].Expect.Judged():
				t.Fatal("a case declaring judge: none loaded as judged")
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("LoadCorpus error = %v, want one naming %q", err, tc.wantErr)
			}
		})
	}
}

// Rendering keeps the declaration, so a probed judge-less case reads back as one.
func TestAJudgelessScenarioRendersBackAsOne(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, dir, "summarize/one.yaml", judgelessScenario("  answer: hi\n  judge: none\n  judge_none_reason: because\n"))
	census := censusFor(t, ai.TaskSummarize)
	loaded, err := aicert.LoadCorpus(dir, census)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	rendered, err := aicert.RenderScenario(loaded[0])
	if err != nil {
		t.Fatalf("RenderScenario: %v", err)
	}
	writeCorpusFile(t, dir, "rendered.yaml", string(rendered))
	back, err := aicert.LoadScenarioFile(dir+"/rendered.yaml", census)
	if err != nil {
		t.Fatalf("LoadScenarioFile: %v", err)
	}
	if back.Expect.Judged() || back.Expect.JudgeNoneReason != "because" || strings.Contains(string(rendered), "bands") {
		t.Fatalf("rendered back as %+v from:\n%s", back.Expect, rendered)
	}
}
