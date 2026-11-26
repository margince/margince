// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/mcpe2e"
)

func TestARouteIsNamedForHowTheAssistantWasReached(t *testing.T) {
	for _, tc := range []struct{ folder, driver, want string }{
		{"claude-sonnet-5-5@claude-cli", "claude:cli", "Claude Code CLI"},
		{"claude-sonnet-5-5@claude-sdk", "claude:sdk", "Claude Agent SDK"},
		{"gpt-5.6-sol@codex-cli", "gpt:cli", "Codex CLI"},
		{"gemini-4-hydrogen-medium@agy-cli", "gemini:cli", "Antigravity CLI"},
		{"mistral-medium-3-5", "mistral:openrouter", "neutral harness, via OpenRouter"},
		{"gpt-5.6-sol", "gpt:api", "neutral harness, vendor API"},
		// Every verdict filed before the lane had routes was driven by the claude CLI.
		{"claude-opus-5", "", "Claude Code CLI"},
	} {
		if got := routeOf(tc.folder, tc.driver); got != tc.want {
			t.Errorf("routeOf(%q, %q) = %q, want %q", tc.folder, tc.driver, got, tc.want)
		}
	}
}

func TestTheTrustVerdictReadsTheShareOfJobsHeld(t *testing.T) {
	for _, tc := range []struct {
		held, tried, cases int
		want               string
	}{
		{21, 21, 21, "🟢 Yes"},
		{17, 21, 21, "🟡 Mostly"},
		{16, 21, 21, "🔴 Not yet"},
		// Tried on under half the jobs: no verdict, whatever the share.
		{1, 1, 21, "⚪ Not enough tested yet"},
		{0, 10, 21, "⚪ Not enough tested yet"},
	} {
		if got := trustVerdict(tc.held, tc.tried, tc.cases); got != tc.want {
			t.Errorf("trustVerdict(%d, %d, %d) = %q, want %q", tc.held, tc.tried, tc.cases, got, tc.want)
		}
	}
}

func TestTheModelNameDropsTheRouteSuffix(t *testing.T) {
	if got := modelName("gpt-5.6-sol@codex-cli"); got != "gpt-5.6-sol" {
		t.Errorf("modelName = %q", got)
	}
}

func TestPlainWordsCountOneJobInTheSingular(t *testing.T) {
	got := plainWords(modelCoverage{CasesRecorded: 1, BelowBar: []string{"case6_ask_the_company"}},
		map[string]string{"case6_ask_the_company": "Ask the company"})
	if want := "Reliable on 0 of 1 job tried; not yet: Ask the company."; got != want {
		t.Errorf("plainWords = %q, want %q", got, want)
	}
}

func TestAJobsCriteriaAreReadWithoutTheirNumbers(t *testing.T) {
	catalog := []criterionRow{{Case: "c", Number: 3, Name: "The record's date wins"}, {Case: "c", Number: 5, Name: "The disagreement is pointed out"}}
	if got, want := plainCriteria("c", []int{3, 5}, catalog), "The record's date wins<br>The disagreement is pointed out"; got != want {
		t.Errorf("plainCriteria = %q, want %q", got, want)
	}
}

// One folder can hold a model's verdicts from more than one route — the vendor
// API and OpenRouter share it — and the page must name every route it averaged.
func TestAModelRunTwoWaysNamesBothRoutes(t *testing.T) {
	cases := []caseRow{
		{Name: "a", ByModel: []caseModelRun{{Model: "gpt-5.6-sol", Runs: 3, Passed: 3, Held: true, Driver: "gpt:api"}}},
		{Name: "b", ByModel: []caseModelRun{{Model: "gpt-5.6-sol", Runs: 3, Passed: 3, Held: true, Driver: "gpt:openrouter"}}},
	}
	if got, want := summariseModel(cases, "gpt-5.6-sol").Route, "neutral harness, vendor API + neutral harness, via OpenRouter"; got != want {
		t.Errorf("Route = %q, want %q", got, want)
	}
}

// Every job on the page is named by its scenario's `title:` line. A reader
// that missed it would render a row with no name and fail nothing.
func TestEveryScenarioNamesItsJob(t *testing.T) {
	cases, err := readE2ELLMCases(e2eLLMScenarioDir, e2eLLMRecordDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("read no scenarios; this census would pass having read nothing")
	}
	for _, c := range cases {
		if c.Title == "" {
			t.Errorf("%s: the page read no `title:` line from it", c.File)
		}
	}
}

// A bridge verdict compares models and a CLI verdict does not, so the two never
// share the main jobs table.
func TestBridgeResultsAreKeptOutOfTheCLIJobsTable(t *testing.T) {
	models := []modelCoverage{
		{Model: "claude-sonnet-5-5@claude-cli"}, {Model: "claude-sonnet-5-5"}, {Model: "gpt-5.6-sol@codex-cli"},
	}
	cli, neutral := splitByHarness(models)
	if len(cli) != 2 || len(neutral) != 1 || neutral[0].Model != "claude-sonnet-5-5" {
		t.Errorf("split = %v / %v, want the two CLI folders apart from the bare one", cli, neutral)
	}
}

func TestAJobNamesTheSeatItRunsAs(t *testing.T) {
	for passport, want := range map[string]string{"": "an admin", "wide": "an admin", "rep": "a rep", "readonly": "read-only"} {
		if got := whoAsks(passport); got != want {
			t.Errorf("whoAsks(%q) = %q, want %q", passport, got, want)
		}
	}
}

// The REST door's verdicts are committed, so the page must read some. A reader
// that found none would publish "not run yet" over results that exist.
func TestTheRESTDoorIsReadFromItsOwnTree(t *testing.T) {
	door, err := readDoor(e2eLLMScenarioDir, e2eLLMRESTDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(door.Models) == 0 || len(door.Cases) == 0 {
		t.Fatalf("read %d models and %d cases from %s; its verdicts are committed", len(door.Models), len(door.Cases), e2eLLMRESTDir)
	}
	for _, m := range door.Models {
		if onNeutralHarness(m.Model) {
			t.Errorf("%s: a REST verdict comes from a CLI, and this folder names none", m.Model)
		}
	}
}

// A case pins the outcome, so a tool a held run used counts as driven, and one
// only a failed run used does not.
func TestAToolAHeldRunCalledIsDriven(t *testing.T) {
	cases := []caseRow{
		{Name: "a", ByModel: []caseModelRun{{Model: "m", Held: true, ToolsCalled: []string{"find_records"}}}},
		{Name: "b", ByModel: []caseModelRun{{Model: "m", Held: false, ToolsCalled: []string{"read_tags"}}}},
	}
	if got := calledInHeldRuns(cases, "find_records"); len(got) != 1 || got[0] != "a" {
		t.Errorf("find_records called in %v, want [a]", got)
	}
	if got := calledInHeldRuns(cases, "read_tags"); len(got) != 0 {
		t.Errorf("read_tags counted from a failed run: %v", got)
	}
}

func TestTheSummaryTableShowsCallsAndTurnsPerJob(t *testing.T) {
	spread := func(median float64) *mcpe2e.Spread {
		return &mcpe2e.Spread{Median: median, Min: int(median), Max: int(median)}
	}
	run := func(model string, calls float64, turns *mcpe2e.Spread) caseModelRun {
		return caseModelRun{
			Model: model, Runs: 2, Passed: 2, PassAt: 2, Held: true,
			Efficiency: &mcpe2e.Efficiency{Runs: 2, Calls: spread(calls), Turns: turns},
		}
	}
	cases := []caseRow{
		{Name: "case1_a", ByModel: []caseModelRun{run("claude-sonnet-5-5@claude-cli", 4, spread(5)), run("gpt-5.6-sol@codex-cli", 9, nil)}},
		{Name: "case2_b", ByModel: []caseModelRun{run("claude-sonnet-5-5@claude-cli", 6, spread(7)), run("gpt-5.6-sol@codex-cli", 11, nil)}},
	}
	r := mcpToolCoverage{Cases: cases, Models: []modelCoverage{
		summariseModel(cases, "claude-sonnet-5-5@claude-cli"), summariseModel(cases, "gpt-5.6-sol@codex-cli"),
	}}
	var p strings.Builder
	writeCoverageAssistants(&p, r)
	page := p.String()
	for _, want := range []string{
		"| Assistant | How we connected it | Jobs tried | Can I trust it? | Calls per job | Turns per job | In plain words |",
		"| `claude-sonnet-5-5` | Claude Code CLI | 2 of 2 | 🟢 Yes | 5 | 6 |",
		// codex reports no turn count, which is a dash and never a 0.
		"| `gpt-5.6-sol` | Codex CLI | 2 of 2 | 🟢 Yes | 10 | - |",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the summary table lacks %q:\n%s", want, page)
		}
	}
}

// A run the staleness check finds out of date is still shown. The job table and
// the assistant's summary mark it as measured on an earlier listing.
func TestARunOnAnEarlierListingIsShownAndMarked(t *testing.T) {
	lines, err := mcpe2e.Report(repoRoot)
	cases, readErr := readE2ELLMCases(e2eLLMScenarioDir, e2eLLMRecordDir)
	if err != nil || readErr != nil {
		t.Fatal(err, readErr)
	}
	marked := map[verdictKey]bool{}
	for _, c := range cases {
		for _, run := range c.ByModel {
			marked[verdictKey{model: run.Model, scenario: c.Name}] = len(run.EarlierListing) > 0
		}
	}
	for _, line := range lines {
		if got := marked[verdictKey{line.Folder, line.Scenario}]; line.Tree == filepath.Base(e2eLLMRecordDir) && got != (len(line.Why) > 0) {
			t.Errorf("%s %s: the report says %v, the page marks it %t", line.Folder, line.Scenario, line.Why, got)
		}
	}
	earlier := caseRow{Name: "a", ByModel: []caseModelRun{{Model: "m", Held: true, EarlierListing: []string{"tool copy"}}}}
	if mark, n := jobMark(earlier, "m"), summariseModel([]caseRow{earlier}, "m").CasesOnEarlierListing; mark != "✅ earlier listing" || n != 1 {
		t.Errorf("the job reads %q and the summary counts %d, want it marked and counted once", mark, n)
	}
}
