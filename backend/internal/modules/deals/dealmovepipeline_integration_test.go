// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// Moving a deal to another pipeline, against a real database. Every row is
// seeded through the real writers.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type movePipelineEnv struct {
	*configEnv
	source, target ids.PipelineID
	sourceStage    ids.StageID
	firstOpen      ids.StageID
	deal           ids.DealID
}

func (e movePipelineEnv) pipelineWith(t *testing.T, stages ...StageInput) ids.PipelineID {
	t.Helper()
	pipeline, err := e.store.CreatePipeline(e.as(), CreatePipelineInput{Name: "Sales " + ids.NewV7().String(), Stages: stages})
	if err != nil {
		t.Fatalf("seeding a pipeline: %v", err)
	}
	return ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
}

func (e movePipelineEnv) stageNamed(t *testing.T, pipeline ids.PipelineID, name string) ids.StageID {
	t.Helper()
	stages, err := e.store.ListStages(e.as(), &pipeline, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("listing the stages: %v", err)
	}
	for _, st := range stages {
		if st.Name == name {
			return ids.From[ids.StageKind](ids.UUID(st.Id))
		}
	}
	t.Fatalf("no stage %q in the pipeline", name)
	return ids.StageID{}
}

func setupMovePipelineEnv(t *testing.T) movePipelineEnv {
	t.Helper()
	e := movePipelineEnv{configEnv: setupConfigEnv(t)}
	e.source = e.pipelineWith(t, StageInput{Name: "Qualified", Position: 0, Semantic: string(SemanticOpen), WinProbability: 10})
	e.target = e.pipelineWith(t,
		StageInput{Name: "Later", Position: 5, Semantic: string(SemanticOpen), WinProbability: 60},
		StageInput{Name: "First", Position: 2, Semantic: string(SemanticOpen), WinProbability: 30})
	e.sourceStage = e.stageNamed(t, e.source, "Qualified")
	e.firstOpen = e.stageNamed(t, e.target, "First")
	owner := ids.From[ids.UserKind](e.admin)
	deal, err := e.store.CreateDeal(e.asDealWriter(), CreateDealInput{
		Name: "Moving deal", PipelineID: e.source, StageID: e.sourceStage, OwnerID: &owner, OwnerExact: true,
	})
	if err != nil {
		t.Fatalf("seeding the deal: %v", err)
	}
	e.deal = ids.From[ids.DealKind](ids.UUID(deal.Id))
	return e
}

type movedRow struct {
	pipeline, stage ids.UUID
	historyPipeline ids.UUID
	historyTo       ids.UUID
	fromPosition    *int
	history         int
}

func (e movePipelineEnv) stored(t *testing.T) movedRow {
	t.Helper()
	var r movedRow
	if err := e.store.Tx(e.as(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(e.as(), `SELECT pipeline_id, stage_id FROM deal WHERE id = $1`, e.deal).Scan(&r.pipeline, &r.stage); err != nil {
			return err
		}
		return tx.QueryRow(e.as(), `
			SELECT pipeline_id_at_change, to_stage_id, from_position_at_change,
			       (SELECT count(*) FROM deal_stage_history WHERE deal_id = $1)
			  FROM deal_stage_history WHERE deal_id = $1 ORDER BY changed_at DESC, id DESC LIMIT 1`, e.deal).
			Scan(&r.historyPipeline, &r.historyTo, &r.fromPosition, &r.history)
	}); err != nil {
		t.Fatalf("reading the stored move: %v", err)
	}
	return r
}

func TestMovingToAnotherPipelineLandsOnItsFirstOpenStageAndUndoReturns(t *testing.T) {
	e := setupMovePipelineEnv(t)
	before := e.stored(t)

	moved, err := e.store.MoveDealToPipeline(e.asDealWriter(), e.deal, MoveDealPipelineInput{PipelineID: e.target})
	if err != nil {
		t.Fatalf("moving the deal: %v", err)
	}
	got := e.stored(t)
	if got.pipeline != e.target.UUID || got.stage != e.firstOpen.UUID {
		t.Fatalf("deal is on pipeline %s stage %s, want the target's first open stage %s", got.pipeline, got.stage, e.firstOpen)
	}
	if got.history != before.history+1 || got.historyTo != e.firstOpen.UUID || got.historyPipeline != e.target.UUID {
		t.Errorf("history row = %+v, want one more row into the first open stage of the target pipeline", got)
	}
	if got.fromPosition != nil {
		t.Errorf("the history row keeps from position %d, but positions of two pipelines do not compare", *got.fromPosition)
	}
	if moved.Probability == nil || moved.Probability.WinProbability != 30 {
		t.Errorf("probability = %+v, want the first open stage's 30", moved.Probability)
	}

	if _, err := e.store.MoveDealToPipeline(e.asDealWriter(), e.deal, MoveDealPipelineInput{PipelineID: e.source, StageID: &e.sourceStage}); err != nil {
		t.Fatalf("undoing the move: %v", err)
	}
	back := e.stored(t)
	if back.pipeline != e.source.UUID || back.stage != e.sourceStage.UUID || back.history != got.history+1 {
		t.Errorf("after the undo the deal is %+v, want its old pipeline and stage and one more history row", back)
	}
}

func TestMovingToAnotherPipelineIsRefusedWhenItCannotHold(t *testing.T) {
	e := setupMovePipelineEnv(t)
	noOpen := e.pipelineWith(t)
	retired := e.pipelineWith(t, StageInput{Name: "Open", Position: 0, Semantic: string(SemanticOpen), WinProbability: 10})
	if err := e.store.ArchivePipeline(e.as(), retired, nil); err != nil {
		t.Fatalf("retiring a pipeline: %v", err)
	}
	won := e.stageNamed(t, e.target, "Won")
	cases := []struct {
		name     string
		in       MoveDealPipelineInput
		wantCode string
	}{
		{"same pipeline", MoveDealPipelineInput{PipelineID: e.source}, "same_pipeline"},
		{"retired pipeline", MoveDealPipelineInput{PipelineID: retired}, "pipeline_retired"},
		{"no open stage", MoveDealPipelineInput{PipelineID: noOpen}, "pipeline_has_no_open_stage"},
		{"closing stage named", MoveDealPipelineInput{PipelineID: e.target, StageID: &won}, "stage_not_open"},
		{"stage of another pipeline", MoveDealPipelineInput{PipelineID: e.target, StageID: &e.sourceStage}, "stage_not_in_pipeline"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := e.store.MoveDealToPipeline(e.asDealWriter(), e.deal, c.in)
			requireFault(t, err, c.wantCode)
			if got := e.stored(t); got.pipeline != e.source.UUID || got.stage != e.sourceStage.UUID {
				t.Errorf("the refused move left the deal on pipeline %s stage %s", got.pipeline, got.stage)
			}
		})
	}
}

func TestAClosedDealStaysInItsPipeline(t *testing.T) {
	e := setupMovePipelineEnv(t)
	lost := e.stageNamed(t, e.source, "Lost")
	code := "other"
	if _, err := e.store.AdvanceDeal(e.asDealWriter(), e.deal, AdvanceDealInput{ToStageID: lost, LostReasonCode: &code}); err != nil {
		t.Fatalf("closing the deal: %v", err)
	}
	_, err := e.store.MoveDealToPipeline(e.asDealWriter(), e.deal, MoveDealPipelineInput{PipelineID: e.target})
	requireFault(t, err, "deal_closed")
}

func TestMovingPipelineNeedsWriteOnTheDeal(t *testing.T) {
	e := setupMovePipelineEnv(t)
	_, err := e.store.MoveDealToPipeline(e.asScopedStranger(), e.deal, MoveDealPipelineInput{PipelineID: e.target})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep without write moved the deal: %v", err)
	}
	if got := e.stored(t); got.pipeline != e.source.UUID {
		t.Errorf("the denied move left the deal on pipeline %s", got.pipeline)
	}
}

func requireFault(t *testing.T, err error, code string) {
	t.Helper()
	var fault interface {
		FieldFault() (string, string, string)
	}
	if !errors.As(err, &fault) {
		t.Fatalf("error = %v, want a field fault with code %s", err, code)
	}
	if _, got, _ := fault.FieldFault(); got != code {
		t.Fatalf("fault code = %s, want %s", got, code)
	}
}
