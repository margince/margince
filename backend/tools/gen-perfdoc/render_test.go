// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden pages under testdata/")

// fixtureRecords is a daily record carrying every stored verdict, beside a
// record written before verdicts were stored.
func fixtureRecords(t *testing.T) map[string]record {
	t.Helper()
	records, err := loadRecords(filepath.Join("testdata", "records"))
	if err != nil {
		t.Fatalf("loading the fixture records: %v", err)
	}
	return records
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s (run with -update to create it): %v", path, err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the render; run with -update and review the diff.\n--- got ---\n%s", path, got)
	}
}

func TestTheBudgetsPageMatchesItsGolden(t *testing.T) {
	assertGolden(t, "performance-budgets.golden.md", render(fixtureRecords(t)))
}

func TestTheBudgetsTableListsTheDailyBudgetsWithTheWorstStoredVerdict(t *testing.T) {
	page := render(fixtureRecords(t))
	for _, want := range []string{
		"| `PERF-8` | Worklist and Home, as the screen calls them | < 1 s | 3100 ms | over budget (#4912) | 2026-10-07 |",
		"| `PERF-9` | Analytics screen | < 300 ms |  | **not measured** |  |",
		"| `PERF-10` | Search as the screen calls it | < 1 s | 420 ms | within budget | 2026-10-07 |",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the budgets table lacks the row\n%s", want)
		}
	}
}

func TestADailyRowIsListedPerSeatWithItsOwnVerdict(t *testing.T) {
	page := render(fixtureRecords(t))
	section := page[strings.Index(page, "## Daily use (`make bench-daily`)"):]
	for _, want := range []string{
		"| `worklist` | rep | 2900 ms | 3100 ms | 1000 ms | 30 | over budget (#4912) |",
		"| `palette_search` | manager | 180 ms | 420 ms | 1000 ms | 30 | within budget |",
		"| `home_digest` | rep |  |  | 1000 ms | 0 | no data |",
		"| `worklist_first_load` | rep | 5200 ms | 5200 ms | 1000 ms | 1 | not gated |",
		"pool wait 1250 ms in all, 48.0 ms at most, over 3200 acquires",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("the daily-use section lacks\n%s", want)
		}
	}
}

func TestARecordWithoutStoredVerdictsRendersAsBefore(t *testing.T) {
	page := render(fixtureRecords(t))
	want := "| `PERF-4` | Save/mutation | < 150 ms server | 151 ms | **OVER BUDGET** | 2026-08-15 |"
	if !strings.Contains(page, want) {
		t.Errorf("an old record's breach must still be computed from its numbers; want\n%s", want)
	}
}

func TestTheHeaderNamesEveryTargetAndTheWeeklyJob(t *testing.T) {
	page := render(fixtureRecords(t))
	header := page[:strings.Index(page, "## The budgets")]
	for _, target := range []string{"bench-perf", "bench-record", "bench-capture", "bench-mobile", "bench-daily", "bench-perf-check", "bench-mobile-check"} {
		if !strings.Contains(header, "`make "+target+"`") && !strings.Contains(header, "`"+target+"`") {
			t.Errorf("the header does not name %s", target)
		}
	}
	if strings.Contains(header, "runs in CI") || strings.Contains(page, "—") {
		t.Error("the header must not claim nothing runs in CI, and the page carries no em dash")
	}
}

func TestTheSummaryRanksByVerdictThenBySpeed(t *testing.T) {
	listedSlow := measurement{ID: "PERF-8", P95Ms: 3100, BudgetMs: 1000, Verdict: storedOverKnown, KnownIssue: 4912}
	unlistedLessSlow := measurement{ID: "PERF-8", P95Ms: 1200, BudgetMs: 1000, Verdict: storedOver}
	firstLoad := measurement{ID: "PERF-8", P95Ms: 9000, BudgetMs: 1000, Samples: 1, Verdict: storedNotGated}
	records := map[string]record{dailyTarget: {Target: dailyTarget, Budgets: []measurement{listedSlow, unlistedLessSlow, firstLoad}}}
	got, ok := findMeasurement(records, published[budgetIndex(t, "PERF-8")])
	if !ok || got.value.P95Ms != 1200 {
		t.Fatalf("got %+v; an unlisted breach outranks a slower listed one, and an ungated row never wins", got.value)
	}
}

func TestAStoredPassNeverHidesACaveat(t *testing.T) {
	m := measurement{P95Ms: 10, BudgetMs: 1000, Verdict: storedWithin, Caveat: "development scale 0.05"}
	if got := verdict(m); got != "inconclusive: development scale 0.05" {
		t.Fatalf("got %q; a development-scale pass proves nothing", got)
	}
}

func TestAnUnknownStoredVerdictIsRefused(t *testing.T) {
	if err := validStoredVerdict(measurement{Name: "worklist", Verdict: "fine"}); err == nil {
		t.Error("an unknown verdict must be refused, or it renders as a pass")
	}
	if err := validStoredVerdict(measurement{Name: "worklist", Verdict: storedOverKnown}); err == nil {
		t.Error("a listed breach without its issue number must be refused")
	}
}

func budgetIndex(t *testing.T, id string) int {
	t.Helper()
	for i, b := range published {
		if b.id == id {
			return i
		}
	}
	t.Fatalf("%s is not a published budget", id)
	return -1
}
