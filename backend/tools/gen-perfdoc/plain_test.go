// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func TestTheBenchmarkPageMatchesItsGolden(t *testing.T) {
	assertGolden(t, "benchmark.golden.md", renderPlain(fixtureRecords(t)))
}

func TestTheBenchmarkPageAnswersInPlainWords(t *testing.T) {
	page := renderPlain(fixtureRecords(t))
	for _, want := range []string{
		"## How fast does my morning start?",
		"🟡 About 3.1 seconds",
		"we are working on it ([#4912](https://github.com/margince/margince/issues/4912))",
		"🟢 Under a quarter of a second",
		"⚪ Not measured",
		"first open after a restart: about 5.2 seconds",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the benchmark page lacks %q", want)
		}
	}
}

func TestTheBenchmarkPageKeepsTechnicalWordsInsideItsDetails(t *testing.T) {
	page := renderPlain(fixtureRecords(t))
	start, end := strings.Index(page, "<details>"), strings.Index(page, "</details>")
	if start < 0 || end < start {
		t.Fatal("the page has no collapsible How this page is made")
	}
	outside := page[:start] + page[end:]
	for _, word := range []string{" ms", "p95", "PERF-", "—"} {
		if strings.Contains(outside, word) {
			t.Errorf("%q appears outside the details block", word)
		}
	}
	if head := strings.Join(strings.SplitN(page, "\n", 6)[:5], "\n"); !strings.Contains(head, "<!-- ") || !strings.Contains(head, "do not edit by hand") {
		t.Error("the first five lines must carry the do-not-edit HTML comment")
	}
}

func TestTheBenchmarkPageWithNoRecordSaysSo(t *testing.T) {
	page := renderPlain(map[string]record{})
	if !strings.Contains(page, "Nobody has measured this yet") || strings.Contains(page, "## How fast does my morning start?") {
		t.Errorf("a checkout with no daily record must say so and show no answers:\n%s", page)
	}
}

func TestATimeIsRoundedTheWayItIsSaidAloud(t *testing.T) {
	for _, c := range []struct {
		ms   float64
		want string
	}{
		{0, "Under a quarter of a second"},
		{249, "Under a quarter of a second"},
		{250, "Under a second"},
		{999, "Under a second"},
		{1000, "About 1 second"},
		{1040, "About 1 second"},
		{3100, "About 3.1 seconds"},
		{9949, "About 9.9 seconds"},
		{12400, "About 12 seconds"},
	} {
		if got := spokenTime(c.ms); got != c.want {
			t.Errorf("spokenTime(%v) = %q, want %q", c.ms, got, c.want)
		}
	}
}

func TestAServerErrorIsRedEvenWithinBudget(t *testing.T) {
	failed := measurement{Name: "contact_360", Seat: "rep", P95Ms: 40, BudgetMs: 300, Samples: 30, Verdict: storedWithin, Status5xx: 2}
	page := renderPlain(map[string]record{dailyTarget: {Target: dailyTarget, Budgets: []measurement{failed}}})
	if !strings.Contains(page, "| A contact's whole story | 🔴 Under a quarter of a second | ⚪ Not measured | Too slow: some tries failed. |") {
		t.Errorf("a 5xx must turn the moment red:\n%s", page)
	}
	if got := verdict(failed); got != "**FAILED**: 2 server errors" {
		t.Errorf("budgets page verdict %q, want the failure named", got)
	}
}

func TestADevelopmentScaleRunIsNeverGreen(t *testing.T) {
	trial := measurement{Name: "lists", Seat: "rep", P95Ms: 40, BudgetMs: 150, Samples: 30, Verdict: storedWithin, Caveat: "development scale 0.05"}
	page := renderPlain(map[string]record{dailyTarget: {Target: dailyTarget, Budgets: []measurement{trial}}})
	if strings.Contains(page, "| A list of 50 records | 🟢") || !strings.Contains(page, "This run is a trial.") {
		t.Errorf("a development-scale pass must not read as fast:\n%s", page)
	}
}
