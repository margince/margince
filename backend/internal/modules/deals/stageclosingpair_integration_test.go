// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// A pipeline keeps a way to win and a way to lose a deal, against a real
// database. Every row is seeded through the real writers.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func closingStageIDs(t *testing.T, e *configEnv, pipelineID ids.PipelineID) (won, lost ids.StageID) {
	t.Helper()
	stages, err := e.store.ListStages(e.as(), &pipelineID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("listing the stages: %v", err)
	}
	for _, st := range stages {
		id := ids.From[ids.StageKind](ids.UUID(st.Id))
		switch StageSemantic(st.Semantic) {
		case SemanticWon:
			won = id
		case SemanticLost:
			lost = id
		}
	}
	return won, lost
}

func requireLastClosingRefusal(t *testing.T, err error, field string) {
	t.Helper()
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != codeLastClosingStage || parse.Field != field {
		t.Fatalf("the refusal is %v, want code %s on field %s", err, codeLastClosingStage, field)
	}
}

func TestThePipelinesOnlyClosingStageCannotBeRetyped(t *testing.T) {
	for _, kind := range []StageSemantic{SemanticWon, SemanticLost} {
		for _, wanted := range []StageSemantic{SemanticOpen, SemanticWon, SemanticLost} {
			if wanted == kind {
				continue
			}
			t.Run(string(kind)+" to "+string(wanted), func(t *testing.T) {
				e := setupConfigEnv(t)
				ctx := e.as()
				pipeline, err := e.store.CreatePipeline(ctx, CreatePipelineInput{
					Name: "Partner " + ids.NewV7().String(), Stages: closingPair(),
				})
				if err != nil {
					t.Fatalf("seeding the pipeline: %v", err)
				}
				won, lost := closingStageIDs(t, e, ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)))
				target := won
				if kind == SemanticLost {
					target = lost
				}
				semantic := string(wanted)
				_, err = e.store.UpdateStage(ctx, target, UpdateStageInput{Semantic: &semantic})
				requireLastClosingRefusal(t, err, stageSemanticField)
				if got := storedStageSemantic(t, e, target); got != kind {
					t.Errorf("the stage now reads %s, so the refusal did not leave it alone", got)
				}
			})
		}
	}
}

func TestOneOfTwoLostStagesCanBeRetyped(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()
	stage := e.seedStage(ctx, t, "Second lost", 0)
	lost := string(SemanticLost)
	if _, err := e.store.UpdateStage(ctx, stage, UpdateStageInput{Semantic: &lost}); err != nil {
		t.Fatalf("adding a second lost stage: %v", err)
	}
	open := string(SemanticOpen)
	if _, err := e.store.UpdateStage(ctx, stage, UpdateStageInput{Semantic: &open}); err != nil {
		t.Fatalf("one of two lost stages refused to reopen: %v", err)
	}
}
