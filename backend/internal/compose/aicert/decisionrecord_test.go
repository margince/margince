// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// A decision record is a record of its own kind: its own path, its own key,
// and never a claim on a completion site. The fields that make it one are all
// omitted from a completion record, so every committed record reads and
// re-writes to the bytes on disk.

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// Every committed record re-encodes to the bytes on disk. A field added to
// Record that a completion record now carries would rewrite every one of them
// on the next run — and a rewritten record reads as a fresh measurement.
func TestACompletionRecordIsByteIdenticalAfterTheDecisionFields(t *testing.T) {
	checked := 0
	err := filepath.WalkDir("records", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return walkErr
		}
		onDisk, err := os.ReadFile(path) // #nosec G304 G122 -- a *.json file under the committed records tree
		if err != nil {
			return err
		}
		var rec aicert.Record
		if err := json.Unmarshal(onDisk, &rec); err != nil {
			return err
		}
		if rec.Task == "" {
			return nil // a use-case verdict filed beside the records, not a record
		}
		reencoded, err := json.MarshalIndent(rec, "", "  ")
		if err != nil {
			return err
		}
		if !bytes.Equal(append(reencoded, '\n'), onDisk) {
			t.Errorf("%s no longer re-encodes to its own bytes: a Record field now reaches a completion record", path)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walking the committed records: %v", err)
	}
	if checked == 0 {
		t.Fatal("no committed record was checked; the walk read nothing")
	}
}

// fxDecisionRecord is a certified decision record on fxSite's task and site.
func fxDecisionRecord() aicert.Record {
	const site = "fx"
	return aicert.Record{
		Task: "rate_extract", Kind: aicert.KindDecision, Site: site,
		Provider: "jev_compatible", Model: "typesafe/jev-1.13", ServedModel: "typesafe/jev-1.13",
		EnvClass: "cloud_frontier", PromptVersion: "p-dec", Verdict: aicert.VerdictCertified, Runs: 3, Passed: 3,
		Decision: &aicert.DecisionStats{Kept: 3, KeptCorrect: 3},
		Scenarios: []aicert.ScenarioRecord{
			{Scenario: "fx_steady", Site: site, Stamp: "d-steady", Verdict: aicert.VerdictCertified, Runs: 3, Passed: 3},
		},
	}
}

// A decision record measured the decision lane, not the site's LLM prompt:
// letting it cover the site's completion row would certify a prompt nobody
// ran, and listing it as unclaimed would call a shipped measurement an orphan.
func TestADecisionRecordNeverClaimsTheLLMSite(t *testing.T) {
	perSite := map[string]map[string]string{"rate_extract/fx": {"fx_steady": "d-steady"}}
	rows, unclaimed := aicert.Readiness(aicert.Census{Sites: []aitasks.Site{fxSite}},
		map[string]string{"rate_extract": "p-dec"}, perSite, []aicert.Record{fxDecisionRecord()})

	if row := fxRow(t, rows); row.Certified {
		t.Errorf("the completion site reads certified by %s; a decision record measured another lane", row.Binding())
	}
	if len(unclaimed) != 0 {
		t.Errorf("the decision record is listed as unclaimed: %+v", unclaimed)
	}

	decisions := aicert.DecisionReadiness(perSite, []aicert.Record{fxDecisionRecord()})
	if len(decisions) != 1 || decisions[0].Status() != aicert.StatusCurrent {
		t.Fatalf("decision readiness = %+v, want the one record, current", decisions)
	}
}

// A decision record and a completion record of one task and binding are two
// measurements, so they need two files and two keys.
func TestADecisionRecordHasItsOwnPathAndKey(t *testing.T) {
	dir := t.TempDir()
	rec := fxDecisionRecord()
	if err := aicert.WriteRecord(dir, rec); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	want := filepath.Join(dir, "rate_extract", "decision_fx_jev_compatible_typesafe_jev-1.13_cloud_frontier.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("the decision record is not at %s: %v", want, err)
	}
	if got := aicert.RecordKey(rec); got != "rate_extract/decision:fx/jev_compatible/typesafe/jev-1.13/cloud_frontier" {
		t.Errorf("RecordKey = %q", got)
	}
	completion := rec
	completion.Kind, completion.Site, completion.Model, completion.Decision = "", "", "", nil
	if aicert.RecordKey(completion) == aicert.RecordKey(rec) {
		t.Error("a completion record and a decision record of one binding share a key")
	}
	loaded, err := aicert.LoadRecords(dir)
	if err != nil || len(loaded) != 1 || loaded[0].Kind != aicert.KindDecision || loaded[0].Decision == nil {
		t.Fatalf("LoadRecords = %+v, %v; want the decision record back whole", loaded, err)
	}
}

// A decision record whose measured scenario moved is stale; one whose site no
// longer ships any of its scenarios measures nothing current.
func TestADecisionRecordGoesStaleWhenItsScenarioMoves(t *testing.T) {
	moved := map[string]map[string]string{"rate_extract/fx": {"fx_steady": "d-NEW"}}
	rows := aicert.DecisionReadiness(moved, []aicert.Record{fxDecisionRecord()})
	if len(rows) != 1 || rows[0].Status() != aicert.StatusStale {
		t.Fatalf("decision readiness = %+v, want the record stale", rows)
	}
	if !strings.Contains(rows[0].Standing.Reason(), "fx_steady") {
		t.Errorf("the stale reason %q does not name the scenario that moved", rows[0].Standing.Reason())
	}

	gone := aicert.DecisionReadiness(map[string]map[string]string{}, []aicert.Record{fxDecisionRecord()})
	if len(gone) != 1 || gone[0].Standing.Measured != 0 {
		t.Fatalf("a record over a site with no decision scenarios reads %+v, want nothing measured", gone)
	}
	if rows := aicert.DecisionReadiness(moved, []aicert.Record{sampleCompletionRecord()}); len(rows) != 0 {
		t.Errorf("a completion record reached decision readiness: %+v", rows)
	}
}

func sampleCompletionRecord() aicert.Record {
	return aicert.Record{Task: string(ai.TaskRateExtract), Provider: "gemini", ServedModel: "m", EnvClass: "eu_hosted"}
}
