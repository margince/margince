// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

func TestARouteIsNamedForHowTheAssistantWasReached(t *testing.T) {
	for _, tc := range []struct{ folder, driver, want string }{
		{"claude-sonnet-5-5@claude-cli", "claude:cli", "Claude Code CLI"},
		{"gpt-5.6-sol@codex-cli", "gpt:cli", "Codex CLI"},
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

// Every job on the page is named by its scenario's first line; one that breaks
// the convention would render a row with no name and fail nothing.
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
			t.Errorf("%s: its first line is not `# CASE <n> — <the job in a contact's words>.`", c.File)
		}
	}
}
