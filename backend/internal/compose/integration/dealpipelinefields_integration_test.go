// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A deal field narrowed to some pipelines, at the store every deal write goes
// through, and the admin settings that narrow it.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const notValidForPipeline = "field_not_valid_for_pipeline"

// secondPipeline is a live pipeline with one open stage, beside the seeded one.
func (f dealCFVFixture) secondPipeline(t *testing.T, name string) (ids.PipelineID, ids.StageID) {
	t.Helper()
	made, err := f.e.Deals.CreatePipeline(f.ctx, deals.CreatePipelineInput{Name: name, Stages: []deals.StageInput{
		{Name: "Open", Position: 1, Semantic: "open", WinProbability: 10},
		{Name: "Won", Position: 2, Semantic: "won", WinProbability: 100},
		{Name: "Lost", Position: 3, Semantic: "lost", WinProbability: 0},
	}})
	if err != nil {
		t.Fatalf("creating pipeline %q: %v", name, err)
	}
	var open ids.StageID
	for _, stage := range *made.Stages {
		if stage.Semantic == "open" {
			open = ids.From[ids.StageKind](ids.UUID(stage.Id))
		}
	}
	return ids.From[ids.PipelineKind](ids.UUID(made.Id)), open
}

func (f dealCFVFixture) createDealIn(t *testing.T, pipeline ids.PipelineID, stage ids.StageID, cf map[string]any) (ids.DealID, error) {
	t.Helper()
	created, err := f.store.CreateDeal(f.ctx, deals.CreateDealInput{
		Name: "Scoped", PipelineID: pipeline, StageID: stage, Source: "manual", CustomFields: cf,
	})
	return dealIDOf(ids.UUID(created.Id)), err
}

func refusedFor(t *testing.T, what string, err error, field string) {
	t.Helper()
	refused, ok := errors.AsType[storekit.CustomFieldErrors](err)
	if !ok || len(refused) != 1 || refused[0].Field != field || refused[0].Code != notValidForPipeline {
		t.Fatalf("%s answered %v, want %s on %s", what, err, notValidForPipeline, field)
	}
}

func TestADealFieldNarrowedToAPipelineIsRefusedOnAnotherAtEveryWrite(t *testing.T) {
	f := setupDealCFV(t)
	other, otherOpen := f.secondPipeline(t, "Partners")
	col := f.defineDealField(t, customfields.FieldSpec{
		Object: "deal", Label: "Partner tier", Type: customfields.TypeText, Source: "manual",
		PipelineIDs: []ids.UUID{f.pipeline.UUID},
	})

	_, err := f.createDealIn(t, other, otherOpen, map[string]any{col: "gold"})
	refusedFor(t, "a create on another pipeline", err, col)
	if _, err := f.createDealIn(t, other, otherOpen, map[string]any{col: nil}); err != nil {
		t.Fatalf("a null on another pipeline must pass: %v", err)
	}
	home, err := f.createDealIn(t, f.pipeline, f.stage, map[string]any{col: "gold"})
	if err != nil {
		t.Fatalf("a create on the field's pipeline: %v", err)
	}

	if _, err := f.store.MoveDealToPipeline(f.ctx, home, deals.MoveDealPipelineInput{PipelineID: other}); err != nil {
		t.Fatalf("moving the deal: %v", err)
	}
	moved, err := f.store.GetDeal(f.ctx, home, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the moved deal: %v", err)
	}
	assertCF(t, moved.AdditionalProperties, col, "gold")

	_, err = f.store.UpdateDeal(f.ctx, home, deals.UpdateDealInput{CustomFields: map[string]any{col: "silver"}})
	refusedFor(t, "a patch on the moved deal", err, col)
	cleared, err := f.store.UpdateDeal(f.ctx, home, deals.UpdateDealInput{Clear: []string{col}, CustomFields: map[string]any{col: nil}})
	if err != nil {
		t.Fatalf("clearing the held value: %v", err)
	}
	assertNoCF(t, cleared.AdditionalProperties, col)
}

func TestAFieldWithNoPipelineListAppliesToEveryPipeline(t *testing.T) {
	f := setupDealCFV(t)
	other, otherOpen := f.secondPipeline(t, "Partners")
	col := f.defineDealField(t, customfields.FieldSpec{Object: "deal", Label: "Region", Type: customfields.TypeText, Source: "manual"})
	for _, target := range []struct {
		pipeline ids.PipelineID
		stage    ids.StageID
	}{{f.pipeline, f.stage}, {other, otherOpen}} {
		if _, err := f.createDealIn(t, target.pipeline, target.stage, map[string]any{col: "north"}); err != nil {
			t.Fatalf("an unscoped field refused a deal: %v", err)
		}
	}
}

func TestThePipelineListOfAFieldIsCheckedWhereItIsSet(t *testing.T) {
	f := setupDealCFV(t)
	retired, _ := f.secondPipeline(t, "Retired")
	if err := f.e.Deals.ArchivePipeline(f.ctx, retired, nil); err != nil {
		t.Fatalf("archiving a pipeline: %v", err)
	}
	contactSpec := customfields.FieldSpec{Object: "contact", Label: "Tier", Type: customfields.TypeText, Source: "manual", PipelineIDs: []ids.UUID{f.pipeline.UUID}}
	for name, spec := range map[string]customfields.FieldSpec{
		"a contact field":      contactSpec,
		"an unknown pipeline":  {Object: "deal", Label: "A", Type: customfields.TypeText, Source: "manual", PipelineIDs: []ids.UUID{ids.New[ids.PipelineKind]().UUID}},
		"an archived pipeline": {Object: "deal", Label: "B", Type: customfields.TypeText, Source: "manual", PipelineIDs: []ids.UUID{retired.UUID}},
	} {
		if _, err := f.svc.Create(f.ctx, spec); !errors.As(err, new(*customfields.ValidationError)) {
			t.Errorf("creating %s answered %v, want a validation error", name, err)
		}
	}

	field, err := f.svc.Create(f.ctx, customfields.FieldSpec{Object: "deal", Label: "Scoped", Type: customfields.TypeText, Source: "manual"})
	if err != nil {
		t.Fatalf("creating the field: %v", err)
	}
	narrow := []ids.UUID{f.pipeline.UUID, f.pipeline.UUID}
	narrowed, err := f.svc.Update(f.ctx, ids.UUID(field.Id), customfields.FieldPatch{PipelineIDs: &narrow}, nil)
	if err != nil || narrowed.PipelineIds == nil || len(*narrowed.PipelineIds) != 1 {
		t.Fatalf("narrowing to one pipeline listed twice answered %v, %v, want one stored id", narrowed.PipelineIds, err)
	}
	every := []ids.UUID{}
	widened, err := f.svc.Update(f.ctx, ids.UUID(field.Id), customfields.FieldPatch{PipelineIDs: &every}, nil)
	if err != nil || widened.PipelineIds != nil {
		t.Fatalf("an empty list answered %v, %v, want null for every pipeline", widened.PipelineIds, err)
	}
}

func TestAPipelineArchivedAfterItWasListedStaysOnTheListUntilRemoved(t *testing.T) {
	f := setupDealCFV(t)
	retiring, _ := f.secondPipeline(t, "Retiring")
	later, _ := f.secondPipeline(t, "Archived before it is added")
	field, err := f.svc.Create(f.ctx, customfields.FieldSpec{
		Object: "deal", Label: "Scoped", Type: customfields.TypeText, Source: "manual",
		PipelineIDs: []ids.UUID{retiring.UUID},
	})
	if err != nil {
		t.Fatalf("creating the field: %v", err)
	}
	for _, id := range []ids.PipelineID{retiring, later} {
		if err := f.e.Deals.ArchivePipeline(f.ctx, id, nil); err != nil {
			t.Fatalf("archiving a pipeline: %v", err)
		}
	}

	kept := []ids.UUID{retiring.UUID, f.pipeline.UUID}
	if _, err := f.svc.Update(f.ctx, ids.UUID(field.Id), customfields.FieldPatch{PipelineIDs: &kept}, nil); err != nil {
		t.Fatalf("adding a live pipeline beside an archived one already listed: %v", err)
	}
	added := []ids.UUID{retiring.UUID, later.UUID}
	if _, err := f.svc.Update(f.ctx, ids.UUID(field.Id), customfields.FieldPatch{PipelineIDs: &added}, nil); !errors.As(err, new(*customfields.ValidationError)) {
		t.Fatalf("adding an archived pipeline answered %v, want a validation error", err)
	}
	removed := []ids.UUID{f.pipeline.UUID}
	if _, err := f.svc.Update(f.ctx, ids.UUID(field.Id), customfields.FieldPatch{PipelineIDs: &removed}, nil); err != nil {
		t.Fatalf("removing the archived pipeline: %v", err)
	}
}
