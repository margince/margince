// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// A pipeline keeps a way to win and a way to lose a deal, against a real
// database. Every row is seeded through the real writers.

import (
	"errors"
	"slices"
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
	t.Parallel()
	for _, kind := range []StageSemantic{SemanticWon, SemanticLost} {
		for _, wanted := range []StageSemantic{SemanticOpen, SemanticWon, SemanticLost} {
			if wanted == kind {
				continue
			}
			t.Run(string(kind)+" to "+string(wanted), func(t *testing.T) {
				e := setupConfigEnv(t)
				ctx := e.as()
				pipeline, err := e.store.CreatePipeline(ctx, CreatePipelineInput{
					Name: "Partner " + ids.NewV7().String(),
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
	t.Parallel()
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

func TestANewPipelineIsBornWithAWonAndALostStage(t *testing.T) {
	t.Parallel()
	open := StageInput{Name: "Intro", Position: 1, Semantic: string(SemanticOpen), WinProbability: 10}
	won := StageInput{Name: "Signed", Position: 2, Semantic: string(SemanticWon), WinProbability: 100}
	for name, tc := range map[string]struct {
		stages    []StageInput
		wantNames []string
	}{
		"no stages":       {nil, []string{"Won", "Lost"}},
		"only open":       {[]StageInput{open}, []string{"Intro", "Won", "Lost"}},
		"won but no lost": {[]StageInput{open, won}, []string{"Intro", "Signed", "Lost"}},
		"an open stage named Won": {
			[]StageInput{{Name: "won", Position: 1, Semantic: string(SemanticOpen), WinProbability: 10}},
			[]string{"won", "Won 2", "Lost"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupConfigEnv(t)
			pipeline, err := e.store.CreatePipeline(e.as(), CreatePipelineInput{Name: "P " + ids.NewV7().String(), Stages: tc.stages})
			if err != nil {
				t.Fatalf("creating the pipeline: %v", err)
			}
			stages, err := e.store.ListStages(e.as(), new(ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))), storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, st := range stages {
				got = append(got, st.Name)
			}
			if !slices.Equal(got, tc.wantNames) {
				t.Errorf("the pipeline was born with %v, want %v", got, tc.wantNames)
			}
		})
	}
}

func TestADealTakesNoStageInARetiredPipeline(t *testing.T) {
	t.Parallel()
	e := setupConfigEnv(t)
	ctx := e.as()
	deal, _, to := twoStagePipeline(t, e)
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(ctx, `SELECT pipeline_id FROM stage WHERE id = $1`, to).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	if err := e.store.ArchivePipeline(ctx, pipelineID, nil); err != nil {
		t.Fatalf("retiring the pipeline: %v", err)
	}
	_, err := e.store.AdvanceDeal(e.asDealWriter(), deal, AdvanceDealInput{ToStageID: to})
	var retired *PipelineRetiredError
	if !errors.As(err, &retired) {
		t.Fatalf("the move answered %v, want a PipelineRetiredError", err)
	}
	if _, code, _ := retired.FieldFault(); code != codePipelineRetired {
		t.Errorf("the refusal carries code %s", code)
	}
	owner := ids.From[ids.UserKind](e.admin)
	if _, err := e.store.CreateDeal(e.asDealWriter(), CreateDealInput{
		Source: "manual",
		Name:   "Late", PipelineID: pipelineID, StageID: to, OwnerID: &owner, OwnerExact: true,
	}); !errors.As(err, &retired) {
		t.Errorf("a deal was born in a retired pipeline: %v", err)
	}
	if _, err := e.store.RestorePipeline(ctx, pipelineID); err != nil {
		t.Fatalf("restoring the pipeline: %v", err)
	}
	if _, err := e.store.AdvanceDeal(e.asDealWriter(), deal, AdvanceDealInput{ToStageID: to}); err != nil {
		t.Errorf("the move after the restore answered %v", err)
	}
}

func dropClosingStages(t *testing.T, e *configEnv, pipelineID ids.PipelineID) {
	t.Helper()
	if _, err := e.owner.Exec(e.as(),
		`DELETE FROM stage WHERE pipeline_id = $1 AND semantic <> 'open'`, pipelineID); err != nil {
		t.Fatalf("leaving the pipeline without a closing pair: %v", err)
	}
}

func liveSemantics(t *testing.T, e *configEnv, pipelineID ids.PipelineID) []string {
	t.Helper()
	stages, err := e.store.ListStages(e.as(), &pipelineID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, st := range stages {
		out = append(out, string(st.Semantic))
	}
	slices.Sort(out)
	return out
}

func TestAPipelineWithoutAClosingPairIsRepairedWhenRestoredOrEdited(t *testing.T) {
	t.Parallel()
	want := []string{"lost", "open", "open", "won"}
	t.Run("restoring", func(t *testing.T) {
		e := setupConfigEnv(t)
		_, from, _ := twoStagePipeline(t, e)
		var pipelineID ids.PipelineID
		if err := e.owner.QueryRow(e.as(), `SELECT pipeline_id FROM stage WHERE id = $1`, from).Scan(&pipelineID); err != nil {
			t.Fatal(err)
		}
		if err := e.store.ArchivePipeline(e.as(), pipelineID, nil); err != nil {
			t.Fatal(err)
		}
		dropClosingStages(t, e, pipelineID)
		if _, err := e.store.RestorePipeline(e.as(), pipelineID); err != nil {
			t.Fatalf("restoring: %v", err)
		}
		if got := liveSemantics(t, e, pipelineID); !slices.Equal(got, want) {
			t.Errorf("the restored pipeline holds %v, want %v", got, want)
		}
	})
	t.Run("editing", func(t *testing.T) {
		e := setupConfigEnv(t)
		_, from, _ := twoStagePipeline(t, e)
		var pipelineID ids.PipelineID
		if err := e.owner.QueryRow(e.as(), `SELECT pipeline_id FROM stage WHERE id = $1`, from).Scan(&pipelineID); err != nil {
			t.Fatal(err)
		}
		dropClosingStages(t, e, pipelineID)
		name := "Discovery renamed"
		if _, err := e.store.UpdateStage(e.as(), from, UpdateStageInput{Name: &name}); err != nil {
			t.Fatalf("editing: %v", err)
		}
		if got := liveSemantics(t, e, pipelineID); !slices.Equal(got, want) {
			t.Errorf("the edited pipeline holds %v, want %v", got, want)
		}
	})
}

// A repair beside an open stage named like the missing closing one numbers the
// stage it adds. The stage name index then has nothing to refuse.
func TestARepairBesideAnOpenStageNamedWonNumbersTheAddedStage(t *testing.T) {
	t.Parallel()
	e := setupConfigEnv(t)
	_, from, _ := twoStagePipeline(t, e)
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(e.as(), `SELECT pipeline_id FROM stage WHERE id = $1`, from).Scan(&pipelineID); err != nil {
		t.Fatal(err)
	}
	dropClosingStages(t, e, pipelineID)
	if _, err := e.owner.Exec(e.as(), `UPDATE stage SET name = 'WON' WHERE id = $1`, from); err != nil {
		t.Fatalf("naming an open stage like the closing one: %v", err)
	}
	semantic := string(SemanticOpen)
	if _, err := e.store.UpdateStage(e.as(), from, UpdateStageInput{Semantic: &semantic}); err != nil {
		t.Fatalf("editing: %v", err)
	}
	if got := liveSemantics(t, e, pipelineID); !slices.Equal(got, []string{"lost", "open", "open", "won"}) {
		t.Fatalf("the edited pipeline holds %v, want the closing pair added", got)
	}
	won, lost := closingStageIDs(t, e, pipelineID)
	for stage, want := range map[ids.StageID]string{won: "Won 2", lost: "Lost"} {
		got, err := e.store.GetStage(e.as(), stage)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != want {
			t.Errorf("the added stage is named %q, want %q", got.Name, want)
		}
	}
}
