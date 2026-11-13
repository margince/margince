// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/labeltranslation"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A stage carries its name per UI language. A names-only save is audited, a
// rename leaves the names alone, and an unknown language is refused.
func TestAStageNamesItselfPerLanguage(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()
	pipeline, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: "Sales"})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := e.store.CreateStage(ctx, CreateStageInput{
		PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), Name: "Closed Won", Position: 1,
		NameTranslations: map[string]string{"vi": "Đã thắng"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := labeltranslation.FromWire(stage.NameTranslations)["vi"]; got != "Đã thắng" {
		t.Fatalf("created vi name = %q, want Đã thắng", got)
	}
	id := ids.From[ids.StageKind](ids.UUID(stage.Id))

	named, err := e.store.UpdateStage(ctx, id, UpdateStageInput{NameTranslations: map[string]string{"de": "Gewonnen"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := labeltranslation.FromWire(named.NameTranslations); got["de"] != "Gewonnen" || got["vi"] != "" {
		t.Errorf("names after a replace = %v, want only de", got)
	}
	before, image := e.auditImages(t, "stage", id.UUID)
	if before[stageNameTranslationsField] == nil || image[stageNameTranslationsField] == nil {
		t.Errorf("the names-only save did not audit the names: before %v, after %v", before, image)
	}

	renamed := "Won"
	after, err := e.store.UpdateStage(ctx, id, UpdateStageInput{Name: &renamed})
	if err != nil {
		t.Fatal(err)
	}
	if got := labeltranslation.FromWire(after.NameTranslations)["de"]; got != "Gewonnen" {
		t.Errorf("a rename changed the de name to %q", got)
	}

	var refused *labeltranslation.Error
	if _, err := e.store.UpdateStage(ctx, id, UpdateStageInput{NameTranslations: map[string]string{"fr": "Gagné"}}); !errors.As(err, &refused) {
		t.Errorf("a language the UI does not ship answered %v, want the translation refusal", err)
	}
}
