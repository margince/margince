// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
	"testing"
)

type scopedStageProvider struct {
	datasource.SystemOfRecordProvider
	pipeline ids.UUID
}

func (p scopedStageProvider) StageSemantic(context.Context, ids.UUID) (string, ids.UUID, error) {
	return dealStatusOpen, p.pipeline, nil
}

func TestStageRulesRequireAnExplicitPopulationWhenConfigured(t *testing.T) {
	pipeline, stage := ids.NewV7(), ids.NewV7()
	valid := map[string]any{"pipeline_id": pipeline.String(), "stage_ids": []string{stage.String()}}
	if err := validateStageChangeParams(false)(valid); err != nil {
		t.Fatal(err)
	}
	for _, params := range []map[string]any{
		{}, {"pipeline_id": pipeline.String()}, {"pipeline_id": pipeline.String(), "stage_ids": []string{}},
		{"pipeline_id": pipeline.String(), "stage_ids": []string{stage.String()}, "rogue": true},
	} {
		if err := validateStageChangeParams(false)(params); err == nil {
			t.Fatalf("accepted incomplete scope %v", params)
		}
	}
	if err := validateStageScope(context.Background(), scopedStageProvider{pipeline: pipeline}, valid); err != nil {
		t.Fatal(err)
	}
	if err := validateStageScope(context.Background(), scopedStageProvider{pipeline: ids.NewV7()}, valid); err == nil {
		t.Fatal("accepted a stage from a different pipeline")
	}
}

func TestStageRulesOnlyMatchSelectedStagesFromTheSelectedPipeline(t *testing.T) {
	pipeline, stage := ids.NewV7(), ids.NewV7()
	raw, err := json.Marshal(stageChangeParams{PipelineID: pipeline, StageIDs: []ids.UUID{stage}})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]string{"to_stage_id": stage.String(), "to_status": dealStatusOpen})
	if err != nil {
		t.Fatal(err)
	}
	ev := workflow.Event{Params: raw, Payload: payload, OwnerID: ids.NewV7(), WorkspaceID: ids.NewV7()}
	ex := Executors{Provider: scopedStageProvider{pipeline: pipeline}, Authority: &fakeAuthzResolver{}}
	if matched, err := (stageChangeCreateTask{ex: ex}).Match(context.Background(), ev); !matched || err != nil {
		t.Fatalf("selected stage match = %v, %v", matched, err)
	}
	ex.Provider = scopedStageProvider{pipeline: ids.NewV7()}
	if matched, err := (stageChangeNotify{ex: ex}).Match(context.Background(), ev); matched || err != nil {
		t.Fatalf("wrong pipeline match = %v, %v", matched, err)
	}
	ev.Payload = json.RawMessage(`{"to_status":"won"}`)
	if matched, err := (stageChangeCreateTask{ex: ex}).Match(context.Background(), ev); matched || err != nil {
		t.Fatalf("closed deal match = %v, %v", matched, err)
	}
	ev.Params = nil
	if matched, err := (stageChangeNotify{}).Match(context.Background(), ev); !matched || err != nil {
		t.Fatalf("legacy rule match = %v, %v", matched, err)
	}
}
