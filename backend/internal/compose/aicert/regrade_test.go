// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// A record is JSON a developer commits, and every page prints its stored verdict.
// So each record graded under today's rule must say what that rule says of its
// rows: a hand-edited verdict or count fails here. A record whose grader third
// moved was graded by an older rule, reads stale, and is not held to this one.
func TestEveryRecordGradedUnderTodaysRuleSaysWhatItsRowsSay(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the census: %v", err)
	}
	corpus, err := LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	records, err := LoadRecords("records")
	if err != nil {
		t.Fatalf("loading the records: %v", err)
	}
	stamps, perSite, err := CurrentStamps(t.Context(), corpus, census)
	if err != nil {
		t.Fatalf("stamping the corpus: %v", err)
	}
	rows, _ := Readiness(Census{Sites: census.All(), Scopes: census.Scopes()}, stamps, perSite, records)
	olderRule := map[string]bool{}
	for _, row := range rows {
		if row.Certified && gradedByAnotherRule(row.Standing) {
			olderRule[RecordKey(row.Record)] = true
		}
	}
	held := 0
	for _, rec := range records {
		if rec.Kind == KindDecision || olderRule[RecordKey(rec)] || hasLegacyRow(rec) {
			continue
		}
		held++
		regraded := rec
		regradeFromRows(&regraded)
		if regraded.Verdict != rec.Verdict || regraded.Runs != rec.Runs || regraded.Passed != rec.Passed {
			t.Errorf("%s says %s, %d of %d; its rows say %s, %d of %d",
				RecordKey(rec), rec.Verdict, rec.Passed, rec.Runs, regraded.Verdict, regraded.Passed, regraded.Runs)
		}
	}
	t.Logf("%d records are graded under today's rule and say what their rows say", held)
}

// gradedByAnotherRule says a scenario the record scored was graded another way
// than this build grades it, or the record predates per-scenario stamps.
func gradedByAnotherRule(s Standing) bool {
	if s.TaskStampOnly {
		return s.Stale
	}
	for _, parts := range s.MovedParts {
		if parts.Grader || (!parts.Case && !parts.Prompt) {
			return true
		}
	}
	return false
}

func hasLegacyRow(rec Record) bool {
	for _, row := range rec.Scenarios {
		if row.legacy() {
			return true
		}
	}
	return false
}

// A verdict edited by hand in a record graded under today's rule is caught.
func TestAHandEditedVerdictDisagreesWithItsRows(t *testing.T) {
	bands := &RowBands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}
	rec := Record{Verdict: VerdictCertified, Runs: 3, Passed: 3, Scenarios: []ScenarioRecord{
		{Scenario: "a", Runs: 3, Passed: 3, JudgeScores: []int{90, 90, 90}, Bands: bands},
	}}
	regraded := rec
	regradeFromRows(&regraded)
	if regraded.Verdict == rec.Verdict {
		t.Fatalf("three runs regraded %s; a hand-written certified on them must disagree", regraded.Verdict)
	}
}
