// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// A pipeline name and a stage name within one pipeline are each held once,
// compared trimmed and ignoring case, against the real indexes.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func requireStageNameTaken(t *testing.T, err error, what string) {
	t.Helper()
	fault, _ := httperr.Classify(err)
	if fault.Status != http.StatusConflict || fault.Code != "stage_name_taken" {
		t.Errorf("%s answered %v (status %d, code %q), want 409 stage_name_taken", what, err, fault.Status, fault.Code)
	}
}

func TestAPipelineNameIsHeldOnce(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()
	suffix := ids.NewV7().String()[24:]
	if _, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: "Sales " + suffix}); err != nil {
		t.Fatalf("seeding the pipeline: %v", err)
	}
	_, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: " SALES " + strings.ToUpper(suffix) + " "})
	if !refusedAs[*PipelineNameTakenError](err) {
		t.Errorf("a second spelling of a live pipeline's name answered %v, want pipeline_name_taken", err)
	}
	other, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: "Renewals " + suffix})
	if err != nil {
		t.Fatalf("seeding a second pipeline: %v", err)
	}
	otherID := ids.From[ids.PipelineKind](ids.UUID(other.Id))
	taken := "sales " + suffix
	if _, err := e.store.UpdatePipeline(ctx, otherID, UpdatePipelineInput{Name: &taken}); !refusedAs[*PipelineNameTakenError](err) {
		t.Errorf("a rename onto a live pipeline's name answered %v, want pipeline_name_taken", err)
	}
	long := strings.Repeat("p", values.TextLabelCharacters+1)
	_, err = e.store.UpdatePipeline(ctx, otherID, UpdatePipelineInput{Name: &long})
	assertLabelTooLong(t, err, "name")
}

func TestAStageNameIsHeldOncePerPipeline(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()
	discovery := e.seedStage(ctx, t, "Discovery", 20)
	current, err := e.store.GetStage(ctx, discovery)
	if err != nil {
		t.Fatal(err)
	}
	pipelineID := ids.From[ids.PipelineKind](ids.UUID(current.PipelineId))

	_, err = e.store.CreateStage(ctx, CreateStageInput{PipelineID: pipelineID, Name: " DISCOVERY ", Position: 2, Semantic: string(SemanticOpen)})
	requireStageNameTaken(t, err, "a second spelling of a stage's name")
	proposal, err := e.store.CreateStage(ctx, CreateStageInput{PipelineID: pipelineID, Name: "Proposal", Position: 2, Semantic: string(SemanticOpen)})
	if err != nil {
		t.Fatalf("seeding a second stage: %v", err)
	}
	proposalID := ids.From[ids.StageKind](ids.UUID(proposal.Id))
	taken := "discovery"
	_, err = e.store.UpdateStage(ctx, proposalID, UpdateStageInput{Name: &taken})
	requireStageNameTaken(t, err, "a rename onto another stage's name")

	long := strings.Repeat("s", values.TextLabelCharacters+1)
	_, err = e.store.CreateStage(ctx, CreateStageInput{PipelineID: pipelineID, Name: long, Position: 3, Semantic: string(SemanticOpen)})
	assertLabelTooLong(t, err, "name")
	_, err = e.store.UpdateStage(ctx, proposalID, UpdateStageInput{Name: &long})
	assertLabelTooLong(t, err, "name")

	_, err = e.store.CreatePipeline(ctx, CreatePipelineInput{
		Name: "Twins " + ids.NewV7().String(),
		Stages: []StageInput{
			{Name: "Intro", Position: 1, Semantic: string(SemanticOpen)},
			{Name: " intro", Position: 2, Semantic: string(SemanticOpen)},
		},
	})
	requireStageNameTaken(t, err, "a new pipeline naming two stages alike")

	// Another pipeline holds its own ladder, so the name is free there.
	e.seedStage(ctx, t, "Discovery", 20)
}
