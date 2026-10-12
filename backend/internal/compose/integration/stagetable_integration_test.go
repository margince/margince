// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The stage settings table reads three facts off each stage: its version, its
// exit criteria counts, and the deals on it.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// listedPipeline reads the one default pipeline off GET /v1/pipelines.
func listedPipeline(t *testing.T, e *apptest.AppEnv) crmcontracts.Pipeline {
	t.Helper()
	var list crmcontracts.PipelineListResponse
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &list); status != http.StatusOK {
		t.Fatalf("listing pipelines → %d", status)
	}
	if len(list.Data) != 1 || list.Data[0].Stages == nil || list.Data[0].Version == nil {
		t.Fatalf("got %+v, want one pipeline carrying its version and stages", list.Data)
	}
	return list.Data[0]
}

// stageVersion answers a stage's version, failing when the wire leaves it out.
func stageVersion(t *testing.T, stage crmcontracts.Stage) int64 {
	t.Helper()
	if stage.Version == nil {
		t.Fatalf("stage %q carries no version", stage.Name)
	}
	return *stage.Version
}

func TestAStageEditAnswersTheVersionItsNextWriteIsGuardedBy(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Stage Ver", "stage-ver@fable.test", "Admin")
	first := (*listedPipeline(t, e).Stages)[0]
	seen := stageVersion(t, first)

	var renamed crmcontracts.Stage
	if status := e.Call(t, "PATCH", "/v1/stages/"+first.Id.String(), AnyMap{"name": "Qualified lead"},
		ifMatch(seen), &renamed); status != http.StatusOK {
		t.Fatalf("renaming on the listed version → %d, want 200", status)
	}
	if stageVersion(t, renamed) <= seen {
		t.Fatalf("a rename left the version at %d, from %d", stageVersion(t, renamed), seen)
	}
	if listed := (*listedPipeline(t, e).Stages)[0]; stageVersion(t, listed) != stageVersion(t, renamed) {
		t.Fatalf("the list reads version %d, the rename answered %d", stageVersion(t, listed), stageVersion(t, renamed))
	}

	var problem AnyMap
	if status := e.Call(t, "PATCH", "/v1/stages/"+first.Id.String(), AnyMap{"name": "Lost the race"},
		ifMatch(seen), &problem); status != http.StatusConflict || problem["code"] != "version_skew" {
		t.Fatalf("renaming on the stale version → %d %v, want 409 version_skew", status, problem)
	}
	if status := e.Call(t, "DELETE", "/v1/stages/"+first.Id.String(), nil,
		ifMatch(seen), &problem); status != http.StatusConflict || problem["code"] != "version_skew" {
		t.Fatalf("removing on the stale version → %d %v, want 409 version_skew", status, problem)
	}
	if status := e.Call(t, "DELETE", "/v1/stages/"+first.Id.String(), nil,
		ifMatch(stageVersion(t, renamed)), nil); status != http.StatusNoContent {
		t.Fatalf("removing on the current version → %d, want 204", status)
	}
}

func TestAReorderMovesTheVersionOfEveryStageItMoves(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Stage Ver Order", "stage-ver-order@fable.test", "Admin")
	before := listedPipeline(t, e)
	stages := *before.Stages
	order := make([]string, 0, len(stages))
	for _, stage := range stages {
		order = append(order, stage.Id.String())
	}
	order[0], order[1] = order[1], order[0]
	if status := e.Call(t, "PUT", "/v1/pipelines/"+before.Id.String()+"/stage-order",
		AnyMap{"stage_ids": order}, ifMatch(int64(*before.Version)), nil); status != http.StatusOK {
		t.Fatalf("swapping the first two stages → %d, want 200", status)
	}
	after := map[string]int64{}
	for _, stage := range *listedPipeline(t, e).Stages {
		after[stage.Id.String()] = stageVersion(t, stage)
	}
	for i, stage := range stages {
		moved := i < 2
		if bumped := after[stage.Id.String()] > stageVersion(t, stage); bumped != moved {
			t.Errorf("stage %q (moved %v) went from version %d to %d", stage.Name, moved, stageVersion(t, stage), after[stage.Id.String()])
		}
	}
}

// stageReads answers one stage as every read of it does: the pipeline list,
// the stage list and the stage itself.
func stageReads(ctx context.Context, t *testing.T, e *Env, pipeline ids.PipelineID, stage ids.StageID) map[string]crmcontracts.Stage {
	t.Helper()
	h := deals.NewHandlers(e.DB(), installseam.Deals())
	call := func(serve func(*httptest.ResponseRecorder, *http.Request), out any) {
		rec := httptest.NewRecorder()
		serve(rec, httptest.NewRequest(http.MethodGet, "/v1/stages", nil).WithContext(ctx))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	reads := map[string]crmcontracts.Stage{}
	var pipelines crmcontracts.PipelineListResponse
	call(func(w *httptest.ResponseRecorder, r *http.Request) {
		h.ListPipelines(w, r, crmcontracts.ListPipelinesParams{})
	}, &pipelines)
	var listed crmcontracts.StageListResponse
	call(func(w *httptest.ResponseRecorder, r *http.Request) {
		h.ListStages(w, r, crmcontracts.ListStagesParams{PipelineId: new(crmcontracts.Id(pipeline.UUID))})
	}, &listed)
	var one crmcontracts.Stage
	call(func(w *httptest.ResponseRecorder, r *http.Request) { h.GetStage(w, r, crmcontracts.Id(stage.UUID)) }, &one)
	reads["getStage"] = one
	for _, p := range pipelines.Data {
		for _, s := range *p.Stages {
			if s.Id == crmcontracts.Id(stage.UUID) {
				reads["listPipelines"] = s
			}
		}
	}
	for _, s := range listed.Data {
		if s.Id == crmcontracts.Id(stage.UUID) {
			reads["listStages"] = s
		}
	}
	if len(reads) != 3 {
		t.Fatalf("the stage is missing from a read: found it in %d of 3", len(reads))
	}
	return reads
}

func TestAStageCountsItsLiveExitCriteriaOnEveryRead(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, won := DealFixture(t, e)
	optional := false
	var added []crmcontracts.StageExitCriterion
	for _, in := range []deals.CreateCriterionInput{
		{StageID: open, Key: "budget", Label: "Budget confirmed", Kind: "buyer_confirmed"},
		{StageID: open, Key: "champion", Label: "Champion named", Kind: "role_identified"},
		{StageID: open, Key: "demo", Label: "Demo held", Kind: "event_held", Required: &optional},
	} {
		criterion, err := e.Deals.CreateStageExitCriterion(admin, in)
		if err != nil {
			t.Fatalf("adding %q: %v", in.Key, err)
		}
		added = append(added, criterion)
	}
	retired := ids.From[ids.ExitCriterionKind](ids.UUID(added[0].Id))
	if err := e.Deals.ArchiveStageExitCriterion(admin, open, retired, nil); err != nil {
		t.Fatalf("archiving a criterion: %v", err)
	}

	for stage, want := range map[ids.StageID]crmcontracts.StageExitCriteriaCounts{
		open: {Required: 1, Optional: 1},
		won:  {Required: 0, Optional: 0},
	} {
		for read, got := range stageReads(admin, t, e, pipeline, stage) {
			if got.ExitCriteria == nil || *got.ExitCriteria != want {
				t.Errorf("%s: stage %q exit_criteria = %+v, want %+v", read, got.Name, got.ExitCriteria, want)
			}
		}
	}
}

func TestAStageCountsTheLiveDealsTheCallerMaySee(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, won := DealFixture(t, e)
	e.SeedDeal(t, "Rep one's deal", pipeline, open, &e.Rep1)
	e.SeedDeal(t, "Rep two's deal", pipeline, open, &e.Rep2)
	archived := ids.From[ids.DealKind](e.SeedDeal(t, "Archived deal", pipeline, open, &e.Rep1))
	if _, err := e.Deals.ArchiveDeal(admin, archived, nil); err != nil {
		t.Fatalf("archiving a deal: %v", err)
	}

	var occupied *deals.StageOccupiedError
	if err := e.Deals.ArchiveStage(admin, open, nil); !errors.As(err, &occupied) {
		t.Fatalf("removing the occupied stage: %v, want stage_occupied", err)
	}
	seat := func(objects map[string]principal.ObjectGrant) context.Context {
		return e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
			RoleKeys: []string{"custom"}, Objects: objects, RowScope: principal.RowScopeOwn,
		})
	}
	readsDeals := seat(map[string]principal.ObjectGrant{"pipeline": {Read: true}, "deal": {Read: true}})
	for stage, want := range map[ids.StageID]int{open: occupied.Count, won: 0} {
		for read, got := range stageReads(readsDeals, t, e, pipeline, stage) {
			if got.DealCount == nil || *got.DealCount != want {
				t.Errorf("%s: an own-scope seat reads deal_count %s on %q, want %d", read, countText(got.DealCount), got.Name, want)
			}
		}
	}
	if occupied.Count != 2 {
		t.Errorf("the removal refused on %d deals, want the 2 live ones", occupied.Count)
	}
	configOnly := seat(map[string]principal.ObjectGrant{"pipeline": {Read: true}})
	for read, got := range stageReads(configOnly, t, e, pipeline, open) {
		if got.DealCount != nil {
			t.Errorf("%s: a seat that may not read deals was told %d deals sit on the stage", read, *got.DealCount)
		}
	}
}
