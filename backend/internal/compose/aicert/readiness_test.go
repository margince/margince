// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// The readiness engine both certification renderers read through: the terminal
// table and the generated docs/reference/ai-certification.md page. What is
// tested here is the judgement — which state a record is in, and what it can
// say about why — never a layout, which belongs to whichever renderer owns it.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// fxSite is the one-shot site every case below is measured on.
var fxSite = aitasks.Site{Task: ai.TaskRateExtract, Variant: "fx", Kind: ai.SiteKindOneShot}

// fxRow returns the one row covering fxSite, so an assertion about a record's
// standing cannot be satisfied by a sibling row.
func fxRow(t *testing.T, rows []aicert.ReadinessRow) aicert.ReadinessRow {
	t.Helper()
	var found []aicert.ReadinessRow
	for _, row := range rows {
		if row.Site == fxSite {
			found = append(found, row)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one row for %s, got %d", fxSite.Variant, len(found))
	}
	return found[0]
}

// A stale row that cannot say WHY is a row a reader has to re-derive by hand,
// and the answer decides what re-certifying costs: one scenario, or a task.
func TestAStaleRecordNamesTheScenarioThatMovedUnderIt(t *testing.T) {
	rec := aicert.Record{
		Task: "rate_extract", Provider: "gemini", ServedModel: "gemini-3.5-flash", EnvClass: "eu_hosted",
		PromptVersion: "p-old", Runs: 6, Passed: 6, Verdict: aicert.VerdictCertified,
		Scenarios: []aicert.ScenarioRecord{
			{Scenario: "fx_moved", Site: "fx", Stamp: "s-old", Verdict: aicert.VerdictCertified, Runs: 3, Passed: 3, ReportedAccepted: 3},
			{Scenario: "fx_steady", Site: "fx", Stamp: "s-steady", Verdict: aicert.VerdictCertified, Runs: 3, Passed: 3, ReportedAccepted: 3},
		},
	}
	perSite := map[string]map[string]string{"rate_extract/fx": {"fx_moved": "s-NEW", "fx_steady": "s-steady"}}

	rows, _ := aicert.Readiness(aicert.Census{Sites: []aitasks.Site{fxSite}},
		map[string]string{"rate_extract": "p-new"}, perSite, []aicert.Record{rec})

	row := fxRow(t, rows)
	if row.Status() != aicert.StatusStale {
		t.Fatalf("a record whose measured scenario moved reads %q, want %q", row.Status(), aicert.StatusStale)
	}
	reason := row.Standing.Reason()
	if !strings.Contains(reason, "fx_moved") {
		t.Errorf("the reason does not name the scenario that moved: %q", reason)
	}
	if strings.Contains(reason, "fx_steady") {
		t.Errorf("the reason blames a scenario that did not move, which prices the re-run too high: %q", reason)
	}
}

// A record written before per-scenario stamps existed can name nothing finer
// than its task stamp. Saying so is the honest answer; inventing a scenario
// name would send a reader to a case that never moved.
func TestALegacyRecordSaysItCanNameNoScenario(t *testing.T) {
	rec := aicert.Record{
		Task: "rate_extract", Provider: "gemini", ServedModel: "gemini-3.5-flash", EnvClass: "eu_hosted",
		PromptVersion: "p-old", Runs: 3, Passed: 3, Verdict: aicert.VerdictCertified,
		Scenarios: []aicert.ScenarioRecord{
			{Scenario: "fx_basic", Site: "fx", Verdict: aicert.VerdictCertified, Runs: 3, Passed: 3, ReportedAccepted: 3},
		},
	}

	rows, _ := aicert.Readiness(aicert.Census{Sites: []aitasks.Site{fxSite}},
		map[string]string{"rate_extract": "p-new"},
		map[string]map[string]string{"rate_extract/fx": {"fx_basic": "s-fx"}}, []aicert.Record{rec})

	row := fxRow(t, rows)
	if row.Status() != aicert.StatusStale {
		t.Fatalf("a legacy record whose task stamp moved reads %q, want %q", row.Status(), aicert.StatusStale)
	}
	if !row.Standing.TaskStampOnly {
		t.Error("the standing does not record that only a task stamp was available to judge")
	}
	if reason := row.Standing.Reason(); !strings.Contains(reason, "task stamp") || strings.Contains(reason, "fx_basic") {
		t.Errorf("the reason should say a task stamp is all it has, and name no scenario: %q", reason)
	}
	if got := row.Coverage(); got != aicert.Unmeasured {
		t.Errorf("a legacy record reports coverage %q, want %q — it has no per-scenario claim to count", got, aicert.Unmeasured)
	}
}

// A scenario the corpus has dropped leaves the record right about what ships —
// it is not stale — but it is not coverage either, because nobody can re-run
// it. Counting it would report a site as fully measured on a case that no
// longer exists.
func TestADroppedScenarioIsNeitherStaleNorCoverage(t *testing.T) {
	rec := aicert.Record{
		Task: "rate_extract", Provider: "gemini", ServedModel: "gemini-3.5-flash", EnvClass: "eu_hosted",
		PromptVersion: "p-old", Runs: 6, Passed: 6, Verdict: aicert.VerdictCertified,
		Scenarios: []aicert.ScenarioRecord{
			{Scenario: "fx_retired", Site: "fx", Stamp: "s-retired", Verdict: aicert.VerdictCertified, Runs: 3, Passed: 3, ReportedAccepted: 3},
			{Scenario: "fx_steady", Site: "fx", Stamp: "s-steady", Verdict: aicert.VerdictCertified, Runs: 3, Passed: 3, ReportedAccepted: 3},
		},
	}
	perSite := map[string]map[string]string{"rate_extract/fx": {"fx_steady": "s-steady"}}

	rows, _ := aicert.Readiness(aicert.Census{Sites: []aitasks.Site{fxSite}},
		map[string]string{"rate_extract": "p-old"}, perSite, []aicert.Record{rec})

	row := fxRow(t, rows)
	if row.Status() != aicert.StatusCurrent {
		t.Errorf("a record whose only change is a scenario the corpus dropped reads %q, want %q", row.Status(), aicert.StatusCurrent)
	}
	if got := row.Coverage(); got != "1/1" {
		t.Errorf("coverage is %q — a dropped scenario must not count as a measured one", got)
	}
	if len(row.Standing.Dropped) != 1 || row.Standing.Dropped[0] != "fx_retired" {
		t.Errorf("the standing does not name the dropped scenario: %v", row.Standing.Dropped)
	}
}

// The named scenarios reach a committed page, so their order must come from the
// data and not from a map walk — otherwise regenerating the page rewrites it
// with no change behind the diff.
func TestTheNamedScenariosAreOrdered(t *testing.T) {
	scenarios := []aicert.ScenarioRecord{}
	current := map[string]string{}
	for _, name := range []string{"fx_delta", "fx_alpha", "fx_charlie", "fx_bravo"} {
		scenarios = append(scenarios, aicert.ScenarioRecord{
			Scenario: name, Site: "fx", Stamp: "s-old", Verdict: aicert.VerdictCertified, Runs: 1, Passed: 1, ReportedAccepted: 1,
		})
		current[name] = "s-new"
	}
	rec := aicert.Record{
		Task: "rate_extract", Provider: "gemini", ServedModel: "gemini-3.5-flash", EnvClass: "eu_hosted",
		PromptVersion: "p-old", Runs: 4, Passed: 4, Verdict: aicert.VerdictCertified, Scenarios: scenarios,
	}

	for range 8 {
		rows, _ := aicert.Readiness(aicert.Census{Sites: []aitasks.Site{fxSite}},
			map[string]string{"rate_extract": "p-new"},
			map[string]map[string]string{"rate_extract/fx": current}, []aicert.Record{rec})
		got := fxRow(t, rows).Standing.Moved
		want := []string{"fx_alpha", "fx_bravo", "fx_charlie", "fx_delta"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("the moved scenarios came back as %v, want them sorted as %v", got, want)
		}
	}
}

// A record that is still current says nothing about why it would be stale.
// A reason printed beside a current row is a warning about nothing.
func TestACurrentRecordOffersNoReason(t *testing.T) {
	standing := aicert.Standing{Measured: 3, Moved: []string{"ignored"}}
	if got := standing.Reason(); got != "" {
		t.Errorf("a record that is not stale offers the reason %q, want none", got)
	}
}

// CurrentStamps is what every row is judged against, and it is the only place
// the per-site split is made — so a bug here reports every row's coverage
// wrongly at once, which is exactly what shipped before the split existed.
func TestCurrentStampsAreSplitPerSiteAndFoldToTheTaskStamp(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	corpus, err := aicert.LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	stamps, perSite, err := aicert.CurrentStamps(context.Background(), corpus, census)
	if err != nil {
		t.Fatalf("CurrentStamps: %v", err)
	}

	// Every scenario reaches exactly its own site's bucket, and no other.
	for _, sc := range corpus {
		key := sc.Task + "/" + sc.Site
		if _, present := perSite[key][sc.Name]; !present {
			t.Errorf("scenario %q is missing from its own site bucket %q", sc.Name, key)
		}
		for other, bucket := range perSite {
			if other == key {
				continue
			}
			if _, leaked := bucket[sc.Name]; leaked {
				t.Errorf("scenario %q leaked into %q — a site would count another site's work as its own", sc.Name, other)
			}
		}
	}

	// And the per-task stamp is still the fold of that task's scenarios, so a
	// record written before per-scenario stamps is judged against the same value
	// it always was.
	byTask := map[string][]aicert.Scenario{}
	for _, sc := range corpus {
		byTask[sc.Task] = append(byTask[sc.Task], sc)
	}
	for task, scenarios := range byTask {
		scoped, stampErr := aicert.ScenarioStamps(context.Background(), scenarios, census)
		if stampErr != nil {
			t.Fatalf("task %s: ScenarioStamps: %v", task, stampErr)
		}
		if want := aicert.FoldScenarioStamps(scoped); stamps[task] != want {
			t.Errorf("task %s: readiness stamp %s is not the fold of its scenarios (%s)", task, stamps[task], want)
		}
	}
}
