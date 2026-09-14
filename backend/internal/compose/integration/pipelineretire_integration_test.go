// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Retiring a pipeline, and putting it back.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// pipelineID is the typed id for a pipeline the wire handed back.
func pipelineIDOf(id ids.UUID) ids.PipelineID { return ids.From[ids.PipelineKind](id) }

// The default pipeline cannot be retired while it is the default, and the
// refusal names a remedy the caller can actually reach.
//
// An installation with no default has no answer for "where does a new deal go",
// and meeting that at deal-creation time is worse than being refused here.
//
// The remedy is asserted, not just the refusal: promoting another pipeline is
// `updatePipeline`, which needs `pipeline:update` — the grant this caller
// already holds. If clearing a refusal took authority the caller lacks, the
// guard would be a trap rather than a door, and the message would be naming a
// way forward nobody can walk.
func TestTheDefaultPipelineCannotBeRetiredUntilAnotherTakesOver(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	standby, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Standby"})
	if err != nil {
		t.Fatalf("seeding the pipeline that will take over: %v", err)
	}
	// The default is set explicitly rather than assumed: a fresh workspace has
	// none, and a refusal asserted against a workspace with no default would
	// pass for the wrong reason — the guard would never have been reached.
	retiring, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Current process"})
	if err != nil {
		t.Fatalf("seeding the pipeline that will be the default: %v", err)
	}
	defaultID := ids.UUID(retiring.Id)
	makeDefault := true
	if _, err := e.Deals.UpdatePipeline(ctx, pipelineIDOf(defaultID),
		deals.UpdatePipelineInput{IsDefault: &makeDefault}); err != nil {
		t.Fatalf("making it the default: %v", err)
	}

	err = e.Deals.ArchivePipeline(ctx, pipelineIDOf(defaultID), nil)

	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != "default_pipeline_not_archivable" {
		t.Fatalf("retiring the default answered %v — an installation with no default has no answer for "+
			"where a new deal goes, and the caller meets that at deal-creation time instead", err)
	}
	if parse.Message == "" {
		t.Error("the refusal carries no message, so the caller is told no and not what to do about it")
	}

	// The remedy works, with the grants the caller already had.
	isDefault := true
	if _, err := e.Deals.UpdatePipeline(ctx, pipelineIDOf(ids.UUID(standby.Id)),
		deals.UpdatePipelineInput{IsDefault: &isDefault}); err != nil {
		t.Fatalf("promoting the standby pipeline: %v — the refusal named a remedy the caller cannot reach", err)
	}
	if err := e.Deals.ArchivePipeline(ctx, pipelineIDOf(defaultID), nil); err != nil {
		t.Fatalf("retiring the former default after another took over: %v", err)
	}
}

// Retiring a pipeline moves no work: the deals on it keep their stage.
//
// This is the half that makes the operation runnable at all. Forcing deals off
// first would turn retiring a pipeline into a bulk migration, and an operation
// nobody dares run retires nothing.
func TestRetiringAPipelineLeavesItsDealsWhereTheyAre(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	retiring, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Old process"})
	if err != nil {
		t.Fatalf("seeding the pipeline to retire: %v", err)
	}
	pipelineID := pipelineIDOf(ids.UUID(retiring.Id))
	probability := 0
	stage, err := e.Deals.CreateStage(ctx, deals.CreateStageInput{
		PipelineID: pipelineID, Name: "Qualified", Position: 0,
		Semantic: "open", WinProbability: &probability,
	})
	if err != nil {
		t.Fatalf("seeding a stage on the pipeline: %v", err)
	}
	stageID := ids.From[ids.StageKind](ids.UUID(stage.Id))
	owner := ids.From[ids.UserKind](e.Rep1)
	deal, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{
		Name: "Mid-flight", PipelineID: pipelineID, StageID: stageID,
		OwnerID: &owner, OwnerExact: true,
	})
	if err != nil {
		t.Fatalf("seeding a deal on the pipeline: %v", err)
	}

	if err := e.Deals.ArchivePipeline(ctx, pipelineID, nil); err != nil {
		t.Fatalf("retiring the pipeline: %v", err)
	}

	after, err := e.Deals.GetDeal(ctx, ids.From[ids.DealKind](ids.UUID(deal.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the deal after its pipeline was retired: %v — retiring a pipeline must not "+
			"take the work on it out of reach", err)
	}
	if after.StageId == nil || ids.UUID(*after.StageId) != ids.UUID(stage.Id) {
		t.Errorf("the deal moved to stage %s, want it left on %s — retiring a pipeline retires a CHOICE, "+
			"and moving work would make it a bulk migration nobody dares run",
			after.StageId, stage.Id)
	}
}

// A retired pipeline can be put back, and restoring one that was never retired
// changes nothing.
//
// The second half is what makes the retry after a lost response safe: the
// caller cannot tell whether the first call landed, and both answers have to
// read the same.
func TestARetiredPipelineCanBePutBack(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	p, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Seasonal"})
	if err != nil {
		t.Fatalf("seeding the pipeline: %v", err)
	}
	id := pipelineIDOf(ids.UUID(p.Id))
	if err := e.Deals.ArchivePipeline(ctx, id, nil); err != nil {
		t.Fatalf("retiring the pipeline: %v", err)
	}

	restored, err := e.Deals.RestorePipeline(ctx, id)
	if err != nil {
		t.Fatalf("restoring the pipeline: %v — a retirement with no undo is a support ticket", err)
	}
	if restored.ArchivedAt != nil {
		t.Errorf("the restored pipeline still reads archived at %v", restored.ArchivedAt)
	}
	live, err := e.Deals.ListPipelines(ctx, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("listing live pipelines: %v", err)
	}
	if !listsPipeline(live, "Seasonal") {
		t.Errorf("the restored pipeline is absent from the live list %v", pipelineNames(live))
	}

	// The retry a caller sends after a lost response.
	if _, err := e.Deals.RestorePipeline(ctx, id); err != nil {
		t.Errorf("restoring an already-live pipeline answered %v, want the same answer as the first "+
			"call — a caller who lost the response cannot tell which one they are making", err)
	}
}
