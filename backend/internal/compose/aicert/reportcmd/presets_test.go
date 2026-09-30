// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// Each preset gets its own table, one row per task, naming the rung that answers
// and the one a failed call falls to with the state of each rung's record.
func TestThePresetReportNamesEachRungAndItsRecordsState(t *testing.T) {
	out := renderPresets([]presetReport{{
		File: "openrouter_cloud_eu.yaml",
		Rungs: []aicert.PresetRungState{
			{
				Task: ai.TaskSummarize, FirstTier: "cheap_cloud", FirstModel: "vendor/a", FirstState: aicert.StatusCurrent,
				FallbackTier: "premium", FallbackModel: "vendor/b", FallbackState: aicert.StatusAbsent,
			},
			{
				Task: ai.TaskColdStart, FirstTier: "cheap_cloud", FirstModel: "vendor/a", FirstState: aicert.StatusStale,
				FallbackState: aicert.RungNone,
			},
		},
	}})
	for _, want := range []string{
		"openrouter_cloud_eu.yaml",
		"summarize", "cheap_cloud · vendor/a · current", "premium · vendor/b · absent",
		"cold_start", "cheap_cloud · vendor/a · stale", "none",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("preset report lacks %q:\n%s", want, out)
		}
	}
}

// A same-model rung is said to be no fallback rather than rendered as a record.
func TestThePresetReportSaysASameModelRungIsNoFallback(t *testing.T) {
	out := renderPresets([]presetReport{{File: "p.yaml", Rungs: []aicert.PresetRungState{
		{
			Task: ai.TaskSummarize, FirstTier: "local_small", FirstModel: "gemma", FirstState: aicert.StatusCurrent,
			FallbackTier: "cheap_cloud", FallbackModel: "gemma", FallbackState: aicert.RungSameModel,
		},
	}}})
	if !strings.Contains(out, "same model") || strings.Contains(out, "cheap_cloud · gemma") {
		t.Errorf("a same-model rung must read \"same model\", not as a record:\n%s", out)
	}
}

const onePreset = `version: 1
seeds:
  ai_routing:
    profile: cloud_frontier
    tiers:
      local_small: { provider: gemini, model: "gemini-3.1-flash-lite" }
      cheap_cloud: { provider: gemini, model: "gemini-3.1-flash-lite" }
      premium:     { provider: gemini, model: "gemini-3.5-flash" }
      frontier:    { provider: gemini, model: "gemini-3.5-flash" }
    embeddings:  { provider: gemini, model: "gemini-embedding-001", dimensions: 768 }
`

// Every preset in the directory is read, and a file that does not parse stops
// the report rather than leaving a preset out of it.
func TestPresetsAreReadWholeOrNotAtAll(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gemini.yaml"), onePreset)
	writeFile(t, filepath.Join(dir, "README.md"), "not a preset")
	presets, err := loadPresets(dir)
	if err != nil || len(presets) != 1 || presets["gemini.yaml"].Profile != ai.ProfileCloudFrontier {
		t.Fatalf("presets = %+v (%v), want gemini.yaml alone under cloud_frontier", presets, err)
	}
	reports, err := presetReports(context.Background(), dir, nil, nil, nil)
	if err != nil || len(reports) != 1 || reports[0].File != "gemini.yaml" || len(reports[0].Rungs) != 0 {
		t.Fatalf("reports = %+v (%v), want gemini.yaml with no task rows for an empty corpus", reports, err)
	}
	writeFile(t, filepath.Join(dir, "broken.yaml"), "seeds: [")
	if _, err := loadPresets(dir); err == nil || !strings.Contains(err.Error(), "broken.yaml") {
		t.Errorf("a preset that does not parse must name itself and stop the report, got %v", err)
	}
	if _, err := loadPresets(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing preset directory must stop the report")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
