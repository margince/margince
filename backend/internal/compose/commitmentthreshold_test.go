// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

import (
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// waivedRecords are certified records the gate does not read, each with the
// reason it cannot be re-measured. A waiver names a provider and a model, never
// a task, so a new record for the same model is read again.
var waivedRecords = gatekit.Waive(map[string]string{
	"vllm/mlx-community/Qwen3-14B-4bit": "needs a local vLLM server on a 24GB Apple-silicon machine, so its record keeps the" +
		" verdict it had before this gate existed and says nothing about how it reads a soft promise",
})

const rerunCertLane = "re-run the cert lane: make -C backend e2e-ai-certify"

// TestCommitmentTaskConfidenceSitsInTheCertifiedGap holds the constant to the
// certification records: every certified record of a task that reads
// commitments must have measured a firm and a hedged promise, and the
// threshold must fall between what it read of each.
func TestCommitmentTaskConfidenceSitsInTheCertifiedGap(t *testing.T) {
	if compose.CommitmentTaskConfidence < compose.ExtractorFloor {
		t.Errorf("the threshold %v is under the extractor floor %v, so it would describe readings that are dropped before the rule sees them",
			compose.CommitmentTaskConfidence, compose.ExtractorFloor)
	}

	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatal(err)
	}
	scenarios, err := aicert.LoadCorpus(filepath.Join("aicert", "corpus"), census)
	if err != nil {
		t.Fatal(err)
	}
	bands := map[string]map[string]string{}
	for _, sc := range scenarios {
		if sc.CommitmentBand == "" {
			continue
		}
		if bands[sc.Task] == nil {
			bands[sc.Task] = map[string]string{}
		}
		bands[sc.Task][sc.Name] = sc.CommitmentBand
	}
	if len(bands) == 0 {
		t.Fatal("no corpus scenario carries a commitment_band, so there is nothing to certify the threshold against")
	}

	defer waivedRecords.AssertAllMatched(t)
	records, err := aicert.LoadRecords(filepath.Join("aicert", "records"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, rec := range records {
		if rec.Kind != "" || rec.Verdict != "certified" || bands[rec.Task] == nil {
			continue
		}
		if waivedRecords.Waived(t, rec.Provider+"/"+rec.ServedModel) {
			continue
		}
		checked++
		firmMin, hedgedMax := commitmentGap(t, rec, bands[rec.Task])
		who := rec.Task + "/" + rec.Provider + "/" + rec.ServedModel
		if firmMin != nil && *firmMin < compose.CommitmentTaskConfidence {
			t.Errorf("%s: a firm promise was read at %v, below the threshold %v, so it would stage instead of becoming a task",
				who, *firmMin, compose.CommitmentTaskConfidence)
		}
		if hedgedMax != nil && *hedgedMax >= compose.CommitmentTaskConfidence {
			t.Errorf("%s: a hedged promise was read at %v, at or above the threshold %v, so it would become a task unasked",
				who, *hedgedMax, compose.CommitmentTaskConfidence)
		}
	}
	if checked == 0 {
		t.Error("no certified record of a commitment-reading task was found")
	}
}

// commitmentGap is the least confidence a record read a firm promise at and the
// greatest it read a hedged one at. A banded scenario the record has no row for,
// or a row that predates the band, is a record that was never measured for this
// gate and fails rather than passing as an absence of evidence.
func commitmentGap(t *testing.T, rec aicert.Record, bands map[string]string) (firmMin, hedgedMax *float64) {
	t.Helper()
	rows := map[string]aicert.ScenarioRecord{}
	for _, row := range rec.Scenarios {
		rows[row.Scenario] = row
	}
	for name, band := range bands {
		who := rec.Task + "/" + rec.Provider + "/" + rec.ServedModel
		row, ok := rows[name]
		if !ok || row.CommitmentBand != band {
			t.Errorf("%s has no measurement of %q — %s", who, name, rerunCertLane)
			continue
		}
		switch band {
		case aicert.CommitmentBandFirm:
			if row.AnswerConfidenceMin == nil {
				t.Errorf("%s: no kept run of the firm scenario %q reported a confidence", who, name)
				continue
			}
			if firmMin == nil || *row.AnswerConfidenceMin < *firmMin {
				firmMin = row.AnswerConfidenceMin
			}
		case aicert.CommitmentBandHedged:
			if row.AnswerConfidenceMax != nil && (hedgedMax == nil || *row.AnswerConfidenceMax > *hedgedMax) {
				hedgedMax = row.AnswerConfidenceMax
			}
		}
	}
	return firmMin, hedgedMax
}
