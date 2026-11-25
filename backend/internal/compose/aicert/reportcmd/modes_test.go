// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

func recordFor(task, site, stamp string) aicert.Record {
	return aicert.Record{
		Task: task, Provider: "anthropic", ServedModel: "claude-sonnet-4-6", EnvClass: "byok",
		PromptVersion: stamp, Verdict: aicert.VerdictCertified, Runs: 3, Passed: 3,
		Scenarios: []aicert.ScenarioRecord{scenarioRecordFor(site)},
	}
}

func twoSites() []aitasks.Site {
	return []aitasks.Site{
		{Task: ai.TaskRateExtract, Variant: "fx", Kind: ai.SiteKindOneShot},
		{Task: ai.TaskBriefRanking, Variant: "rank", Kind: ai.SiteKindOneShot},
	}
}

// The base answer is read back from the text the report prints. A column moved
// in the table breaks this round trip, and not a comment in a pull request.
func TestARecordCurrentInTheBaseReportAndStaleNowIsNamed(t *testing.T) {
	sites := twoSites()
	records := []aicert.Record{recordFor("rate_extract", "fx", currentStamp), recordFor("brief_ranking", "rank", currentStamp)}
	allCurrent := map[string]string{"rate_extract": currentStamp, "brief_ranking": currentStamp}
	base := renderReadiness(aicert.Census{Sites: sites}, allCurrent, nil, records, nil)

	oneMoved := map[string]string{"rate_extract": currentStamp, "brief_ranking": staleStamp}
	rows, _ := aicert.Readiness(aicert.Census{Sites: sites}, oneMoved, nil, records)
	got := newlyStale(statusesOf(base), rows)
	if len(got) != 1 || !strings.HasPrefix(got[0], "brief_ranking/rank | anthropic | claude-sonnet-4-6 | byok") || !strings.HasSuffix(got[0], "now stale") {
		t.Fatalf("newlyStale = %q, want the one record that moved", got)
	}
	comment := renderNewlyStale(got)
	if !strings.Contains(comment, "makes 1 AI certification records stale") || !strings.Contains(comment, "- brief_ranking/rank") {
		t.Errorf("the comment does not name the count and the record:\n%s", comment)
	}
}

func TestAnUnchangedTreeMakesNothingStaleAndSaysNothing(t *testing.T) {
	sites := twoSites()
	records := []aicert.Record{recordFor("rate_extract", "fx", currentStamp), recordFor("brief_ranking", "rank", staleStamp)}
	stamps := map[string]string{"rate_extract": currentStamp, "brief_ranking": currentStamp}
	base := renderReadiness(aicert.Census{Sites: sites}, stamps, nil, records, nil)
	rows, _ := aicert.Readiness(aicert.Census{Sites: sites}, stamps, nil, records)
	// A record already stale in the base report is no newcomer.
	if got := newlyStale(statusesOf(base), rows); len(got) != 0 {
		t.Errorf("newlyStale = %q, want none", got)
	}
	if comment := renderNewlyStale(nil); comment != "" {
		t.Errorf("an empty list renders %q, want no comment", comment)
	}
}

func TestTotalsCountEveryStatus(t *testing.T) {
	sites := twoSites()
	records := []aicert.Record{recordFor("rate_extract", "fx", currentStamp)}
	stamps := map[string]string{"rate_extract": currentStamp, "brief_ranking": currentStamp}
	rows, _ := aicert.Readiness(aicert.Census{Sites: sites}, stamps, nil, records)
	want := "AI certification records: 1 current, 0 partial, 0 stale, 1 absent."
	if got := totals(rows); got != want {
		t.Errorf("totals = %q, want %q", got, want)
	}
}

func TestAPresetRungThatIsNotCurrentIsListedAndAFallbackThatIsTheSameModelIsNot(t *testing.T) {
	report := presetReport{File: "p.yaml", Rungs: []aicert.PresetRungState{
		{Task: ai.TaskRateExtract, FirstTier: ai.TierPremium, FirstModel: "m", FirstState: aicert.StatusCurrent, FallbackState: aicert.RungSameModel},
		{Task: ai.TaskBriefRanking, FirstTier: ai.TierPremium, FirstModel: "m", FirstState: aicert.StatusStale, FallbackState: aicert.RungNone},
		{
			Task: ai.TaskAgentLoop, FirstTier: ai.TierPremium, FirstModel: "m", FirstState: aicert.StatusCurrent,
			FallbackTier: ai.TierCheapCloud, FallbackModel: "n", FallbackState: aicert.StatusAbsent,
		},
	}}
	got := notCurrentRungs(report)
	want := []string{"brief_ranking: premium on m is stale", "agent_loop: fallback cheap_cloud on n is absent"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("notCurrentRungs = %q, want %q", got, want)
	}
}

// A record the changed tree no longer carries has no row to compare, so the
// base's own binding names it.
func TestARecordTheChangedTreeNoLongerCarriesIsNamedAsGone(t *testing.T) {
	sites := twoSites()
	records := []aicert.Record{recordFor("rate_extract", "fx", currentStamp), recordFor("brief_ranking", "rank", currentStamp)}
	stamps := map[string]string{"rate_extract": currentStamp, "brief_ranking": currentStamp}
	base := renderReadiness(aicert.Census{Sites: sites}, stamps, nil, records, nil)

	rows, _ := aicert.Readiness(aicert.Census{Sites: sites}, stamps, nil, records[:1])
	got := newlyStale(statusesOf(base), rows)
	if len(got) != 1 || got[0] != "brief_ranking/rank | anthropic | claude-sonnet-4-6 | byok: now gone" {
		t.Errorf("newlyStale = %q, want the deleted record named by its base binding", got)
	}
}
