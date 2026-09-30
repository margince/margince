// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Reordering a pipeline's ladder and the pipeline catalog, each named whole:
// the order lands as positions 1..n in one write, is guarded by the pipeline's
// version, publishes one fact, and refuses an order it cannot trust.

import (
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// orderedPipeline is the slice of a pipeline response a reorder is about.
type orderedPipeline struct {
	ID      string            `json:"id"`
	Version int64             `json:"version"`
	Stages  []configuredStage `json:"stages"`
}

func TestStageReorderMovesTheLadderInOneWrite(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Stage Order", "stage-order@fable.test", "Admin")
	seeded := apptest.DiscoverSeededPipeline(t, e)
	before := readOrderedPipeline(t, e, seeded.PipelineID)
	if len(before.Stages) < 4 {
		t.Fatalf("the seeded ladder has %d stages; this scenario moves the third open stage to the top", len(before.Stages))
	}

	// The third stage to the top: as single PATCHes, the very first step
	// collides with the stage already at position 1.
	ids := stageIDs(before.Stages)
	moved := append([]string{ids[2], ids[0], ids[1]}, ids[3:]...)
	var after orderedPipeline
	if status := e.Call(t, "PUT", "/v1/pipelines/"+seeded.PipelineID+"/stage-order",
		AnyMap{"stage_ids": moved}, ifMatch(before.Version), &after); status != http.StatusOK {
		t.Fatalf("reordering the ladder → %d, want 200", status)
	}
	if got := stageIDs(after.Stages); !slices.Equal(got, moved) {
		t.Fatalf("the answered ladder is %v, want %v", got, moved)
	}
	assertContiguous(t, e, seeded.PipelineID, len(moved))
	if after.Version <= before.Version {
		t.Fatalf("the pipeline's version went %d → %d; the ladder's version must move with its order",
			before.Version, after.Version)
	}
	// Three stages moved, and they ride ONE pipeline fact naming exactly them.
	assertReorderFacts(t, e, seeded.PipelineID, 1, map[string]int{ids[2]: 1, ids[0]: 2, ids[1]: 3})

	// The order drawn before the reorder is now stale, and refuses.
	var problem AnyMap
	if status := e.Call(t, "PUT", "/v1/pipelines/"+seeded.PipelineID+"/stage-order",
		AnyMap{"stage_ids": ids}, ifMatch(before.Version), &problem); status != http.StatusConflict ||
		problem["code"] != "version_skew" {
		t.Fatalf("reordering on the stale version → %d %v, want 409 version_skew", status, problem)
	}
	// Restating the order it already has moves nothing, records nothing, and
	// leaves the version where it was.
	var unchanged orderedPipeline
	if status := e.Call(t, "PUT", "/v1/pipelines/"+seeded.PipelineID+"/stage-order",
		AnyMap{"stage_ids": moved}, ifMatch(after.Version), &unchanged); status != http.StatusOK {
		t.Fatalf("restating the current order → %d, want 200", status)
	}
	if unchanged.Version != after.Version {
		t.Fatalf("a reorder that moved nothing moved the version %d → %d", after.Version, unchanged.Version)
	}
	assertReorderFacts(t, e, seeded.PipelineID, 1, map[string]int{ids[2]: 1, ids[0]: 2, ids[1]: 3})
}

func TestStageReorderRefusesAnOrderItCannotTrust(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Stage Order Guard", "stage-order-guard@fable.test", "Admin")
	seeded := apptest.DiscoverSeededPipeline(t, e)
	ladder := stageIDs(readOrderedPipeline(t, e, seeded.PipelineID).Stages)
	path := "/v1/pipelines/" + seeded.PipelineID + "/stage-order"

	// A closing stage above an open one would put a deal's end in its middle.
	openFirst := slices.DeleteFunc(slices.Clone(ladder), func(id string) bool { return id == seeded.Won })
	closingFirst := append([]string{seeded.Won}, openFirst...)
	assertFieldRefusal(t, e, path, AnyMap{"stage_ids": closingFirst}, "closing_stage_before_open")
	assertFieldRefusal(t, e, path, AnyMap{"stage_ids": append(slices.Clone(ladder), ladder[0])}, "duplicate")
	assertFieldRefusal(t, e, path, AnyMap{}, "required")

	// An order missing a live stage was drawn from an older ladder.
	var problem AnyMap
	if status := e.Call(t, "PUT", path, AnyMap{"stage_ids": ladder[1:]}, nil, &problem); status != http.StatusConflict ||
		problem["code"] != "order_stale" {
		t.Fatalf("an order missing a stage → %d %v, want 409 order_stale", status, problem)
	}
	// A stage added since the read makes the old full list stale too, and the
	// add itself moved the version the reorder is guarded by.
	before := readOrderedPipeline(t, e, seeded.PipelineID)
	if status := e.Call(t, "POST", "/v1/stages", AnyMap{
		"pipeline_id": seeded.PipelineID, "name": "Pilot", "position": len(ladder) + 1,
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("adding a stage → %d, want 201", status)
	}
	if status := e.Call(t, "PUT", path, AnyMap{"stage_ids": ladder}, nil, &problem); status != http.StatusConflict ||
		problem["code"] != "order_stale" {
		t.Fatalf("an order missing the new stage → %d %v, want 409 order_stale", status, problem)
	}
	grown := readOrderedPipeline(t, e, seeded.PipelineID)
	if grown.Version <= before.Version {
		t.Fatalf("adding a stage left the pipeline's version at %d; an order guarded by it would land anyway", grown.Version)
	}
	// Created past the closing pair, the open stage went in front of it.
	if last := grown.Stages[len(grown.Stages)-3]; last.Name != "Pilot" {
		t.Fatalf("the ladder ends %v; the new open stage belongs just before the closing pair", stageIDs(grown.Stages))
	}
	assertContiguous(t, e, seeded.PipelineID, len(ladder)+1)
}

// The ladder keeps its closing stages last and its version moving through
// every write that reshapes it, not only through a reorder.
func TestEveryLadderWriteKeepsItsShapeAndMovesItsVersion(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Ladder Shape", "ladder-shape@fable.test", "Admin")
	seeded := apptest.DiscoverSeededPipeline(t, e)
	version := readOrderedPipeline(t, e, seeded.PipelineID).Version
	moves := func(what string, want bool) {
		t.Helper()
		now := readOrderedPipeline(t, e, seeded.PipelineID).Version
		if (now != version) != want {
			t.Fatalf("%s moved the pipeline's version %d → %d, want moved=%v", what, version, now, want)
		}
		version = now
	}
	patch := func(stageID string, body AnyMap) (int, AnyMap) {
		var answer AnyMap
		return e.Call(t, "PATCH", "/v1/stages/"+stageID, body, nil, &answer), answer
	}

	// A position change that keeps the shape lands and moves the version.
	if status, _ := patch(seeded.Open, AnyMap{"position": 0}); status != http.StatusOK {
		t.Fatalf("moving an open stage to the top → %d, want 200", status)
	}
	moves("a position change", true)
	// Re-sending the position it holds is no change at all.
	if status, _ := patch(seeded.Open, AnyMap{"position": 0}); status != http.StatusOK {
		t.Fatalf("re-sending a stage's own position → %d, want 200", status)
	}
	moves("a position that did not change", false)
	// Moving a closing stage above the open run is refused and moves nothing.
	status, answer := patch(seeded.Won, AnyMap{"position": 1})
	fields, _ := answer["details"].(map[string]any)["errors"].([]any)
	if status != http.StatusUnprocessableEntity || len(fields) != 1 ||
		fields[0].(map[string]any)["code"] != "closing_stage_before_open" {
		t.Fatalf("won above the open run → %d %v, want 422 closing_stage_before_open", status, answer)
	}
	moves("a refused change", false)
	// Turning the first stage into a closing one slides it behind the open run.
	if status, answer := patch(seeded.Open, AnyMap{"semantic": "lost"}); status != http.StatusOK {
		t.Fatalf("the first stage to lost → %d %v, want 200", status, answer)
	}
	moves("a stage-type change", true)
	flipped := readOrderedPipeline(t, e, seeded.PipelineID).Stages
	if at := slices.Index(stageIDs(flipped), seeded.Open); at != len(flipped)-3 {
		t.Fatalf("the stage turned lost sits at %d of %v; it belongs just before the closing pair", at, stageIDs(flipped))
	}
	assertContiguous(t, e, seeded.PipelineID, len(flipped))
	if status, _ := patch(seeded.Open, AnyMap{"position": -1}); status != http.StatusUnprocessableEntity {
		t.Fatalf("a negative position → %d, want 422", status)
	}
	// Removing a stage moves the version too.
	stages := readStages(t, e, seeded.PipelineID)
	if status := e.Call(t, "DELETE", "/v1/stages/"+stages[1].ID, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("removing an empty open stage → %d, want 204", status)
	}
	moves("a removal", true)
}

func TestPipelineReorderSetsTheCatalogOrder(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Pipeline Order", "pipeline-order@fable.test", "Admin")
	seeded := apptest.DiscoverSeededPipeline(t, e)
	var created struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/pipelines", AnyMap{"name": "Renewals"}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create a second pipeline → %d", status)
	}

	wanted := []string{created.ID, seeded.PipelineID}
	var listed struct {
		Data []orderedPipeline `json:"data"`
	}
	if status := e.Call(t, "PUT", "/v1/pipelines/order", AnyMap{"pipeline_ids": wanted}, nil, &listed); status != http.StatusOK {
		t.Fatalf("reordering the pipelines → %d, want 200", status)
	}
	if got := pipelineIDs(listed.Data); !slices.Equal(got, wanted) {
		t.Fatalf("the answered catalog is %v, want %v", got, wanted)
	}
	// Every picker reads listPipelines, so the order has to hold there too.
	var catalog struct {
		Data []orderedPipeline `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &catalog); status != http.StatusOK ||
		!slices.Equal(pipelineIDs(catalog.Data), wanted) {
		t.Fatalf("listPipelines after the reorder → %d %v, want %v", status, pipelineIDs(catalog.Data), wanted)
	}

	var problem AnyMap
	if status := e.Call(t, "PUT", "/v1/pipelines/order", AnyMap{"pipeline_ids": wanted[:1]}, nil, &problem); status != http.StatusConflict ||
		problem["code"] != "order_stale" {
		t.Fatalf("an order missing a live pipeline → %d %v, want 409 order_stale", status, problem)
	}
}

// assertReorderFacts holds that the pipeline's reorders published exactly
// `events` pipeline.updated facts, and that together they name exactly the
// stages that moved — counted, because a merged delta cannot tell one fact from
// one per stage.
func assertReorderFacts(t *testing.T, e *apptest.AppEnv, pipelineID string, events int, want map[string]int) {
	t.Helper()
	var published int
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT count(*) FROM event_outbox
		  WHERE envelope->>'type' = 'pipeline.updated' AND envelope->'entity'->>'id' = $1::text
		    AND envelope->'payload'->'changed_fields'->'stage_positions' IS NOT NULL`,
		pipelineID).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published != events {
		t.Fatalf("%d reorder facts for the pipeline, want %d", published, events)
	}
	rows, err := e.Owner.Query(t.Context(),
		`SELECT key, value::int FROM event_outbox,
		        jsonb_each_text(envelope->'payload'->'changed_fields'->'stage_positions')
		  WHERE envelope->>'type' = 'pipeline.updated' AND envelope->'entity'->>'id' = $1::text`, pipelineID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var id string
		var position int
		if err := rows.Scan(&id, &position); err != nil {
			t.Fatal(err)
		}
		got[id] = position
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("the reorder facts name %v, want exactly %v", got, want)
	}
	for id, position := range want {
		if got[id] != position {
			t.Fatalf("the reorder facts name %v, want exactly %v", got, want)
		}
	}
}

func assertFieldRefusal(t *testing.T, e *apptest.AppEnv, path string, body AnyMap, code string) {
	t.Helper()
	var problem struct {
		Details struct {
			Errors []struct {
				Code string `json:"code"`
			} `json:"errors"`
		} `json:"details"`
	}
	status := e.Call(t, "PUT", path, body, nil, &problem)
	if refusals := problem.Details.Errors; status != http.StatusUnprocessableEntity || len(refusals) != 1 || refusals[0].Code != code {
		t.Fatalf("PUT %s → %d %+v, want 422 %s", path, status, problem, code)
	}
}

func readOrderedPipeline(t *testing.T, e *apptest.AppEnv, pipelineID string) orderedPipeline {
	t.Helper()
	var p orderedPipeline
	if status := e.Call(t, "GET", "/v1/pipelines/"+pipelineID, nil, nil, &p); status != http.StatusOK {
		t.Fatalf("reading the pipeline → %d", status)
	}
	return p
}

func stageIDs(stages []configuredStage) []string {
	out := make([]string, len(stages))
	for i, s := range stages {
		out[i] = s.ID
	}
	return out
}

func pipelineIDs(pipelines []orderedPipeline) []string {
	out := make([]string, len(pipelines))
	for i, p := range pipelines {
		out[i] = p.ID
	}
	return out
}
