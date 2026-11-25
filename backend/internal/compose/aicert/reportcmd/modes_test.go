// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
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

func TestTheBaseReportIsReadFromAFileAndAMissingOneIsAnError(t *testing.T) {
	sites := twoSites()
	records := []aicert.Record{recordFor("rate_extract", "fx", currentStamp), recordFor("brief_ranking", "rank", currentStamp)}
	stamps := map[string]string{"rate_extract": currentStamp, "brief_ranking": currentStamp}
	path := filepath.Join(t.TempDir(), "base.txt")
	if err := os.WriteFile(path, []byte(renderReadiness(aicert.Census{Sites: sites}, stamps, nil, records, nil)), 0o600); err != nil {
		t.Fatal(err)
	}
	moved := map[string]string{"rate_extract": staleStamp, "brief_ranking": currentStamp}
	rows, _ := aicert.Readiness(aicert.Census{Sites: sites}, moved, nil, records)
	comment, err := newlyStaleFromFile(path, rows)
	if err != nil || !strings.Contains(comment, "rate_extract/fx") {
		t.Errorf("comment = %q, err = %v, want the moved record named", comment, err)
	}
	if _, err := newlyStaleFromFile(filepath.Join(t.TempDir(), "absent.txt"), rows); err == nil {
		t.Error("a missing base report is no error")
	}
}

func TestARequiredPresetPassesOnlyWhenEveryRungIsCurrentAndItIsThere(t *testing.T) {
	current := presetReport{File: "a.yaml", Rungs: []aicert.PresetRungState{
		{Task: ai.TaskRateExtract, FirstState: aicert.StatusCurrent, FallbackState: aicert.RungNone},
	}}
	behind := presetReport{File: "b.yaml", Rungs: []aicert.PresetRungState{
		{Task: ai.TaskRateExtract, FirstTier: ai.TierPremium, FirstModel: "m", FirstState: aicert.StatusStale, FallbackState: aicert.RungNone},
	}}
	reports := []presetReport{current, behind}
	if problems, code := requireCurrent(reports, "a.yaml"); code != 0 || len(problems) != 0 {
		t.Errorf("a current preset = %q, %d, want a pass", problems, code)
	}
	if problems, code := requireCurrent(reports, "b.yaml"); code != 1 || len(problems) != 1 {
		t.Errorf("a stale preset = %q, %d, want one line and a failure", problems, code)
	}
	if problems, code := requireCurrent(reports, "c.yaml"); code != 1 || !strings.Contains(problems[0], "c.yaml") {
		t.Errorf("a missing preset = %q, %d, want a failure naming it", problems, code)
	}
}

func TestTheAnswerFlagsGiveTheirTextAndTheirExitCode(t *testing.T) {
	sites := twoSites()
	records := []aicert.Record{recordFor("rate_extract", "fx", currentStamp)}
	stamps := map[string]string{"rate_extract": currentStamp, "brief_ranking": currentStamp}
	census := aicert.Census{Sites: sites}

	out, problems, code := answer("", "", true, "", census, stamps, nil, records, nil, nil)
	if code != 0 || len(problems) != 0 || !strings.HasPrefix(out, "AI certification records: ") || !strings.HasSuffix(out, "\n") {
		t.Errorf("-totals = %q, %q, %d", out, problems, code)
	}
	if out, problems, code = answer(filepath.Join(t.TempDir(), "absent.txt"), "", false, "", census, stamps, nil, records, nil, nil); code != 1 ||
		out != "" || len(problems) != 1 || !strings.Contains(problems[0], "base report") {
		t.Errorf("-newly-stale with no file = %q, %q, %d", out, problems, code)
	}
	if _, problems, code = answer("", "a.yaml", false, filepath.Join(t.TempDir(), "absent"), census, stamps, nil, records, nil, nil); code != 1 || len(problems) != 1 {
		t.Errorf("-require-current with no presets = %q, %d, want one line and a failure", problems, code)
	}
}
