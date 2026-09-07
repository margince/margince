// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// A stage's exit criteria against a real database.
//
// The claims here are about constraints and transactions that only Postgres
// can answer: a partial unique index that frees a key on archive, a refusal
// that must fire before the CHECK does, and a renumbering that has to leave
// the surviving positions contiguous. Every row is seeded through the real
// writers, so a green here says the shipped path behaves, not that the test
// can write SQL.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// openStage seeds a pipeline with one open stage and answers the stage id.
func openStage(t *testing.T, e *configEnv, semantic StageSemantic) ids.StageID {
	t.Helper()
	ctx := e.as()
	pipeline, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: "Sales " + ids.NewV7().String()})
	if err != nil {
		t.Fatalf("seeding a pipeline: %v", err)
	}
	probability := 0
	if semantic == SemanticWon {
		probability = 100
	}
	stage, err := e.store.CreateStage(ctx, CreateStageInput{
		PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)),
		Name:       "Qualified", Position: 0,
		Semantic:       string(semantic),
		WinProbability: &probability,
	})
	if err != nil {
		t.Fatalf("seeding a %s stage: %v", semantic, err)
	}
	return ids.From[ids.StageKind](ids.UUID(stage.Id))
}

func addCriterion(t *testing.T, e *configEnv, stageID ids.StageID, key string) crmcontracts.StageExitCriterion {
	t.Helper()
	c, err := e.store.CreateStageExitCriterion(e.as(), CreateCriterionInput{
		StageID: stageID, Key: key, Label: "Buyer confirmed the problem",
		Kind: string(CriterionBuyerConfirmed),
	})
	if err != nil {
		t.Fatalf("adding criterion %s: %v", key, err)
	}
	return c
}

// A stage's criteria are unique by key, so an extractor citing a key names
// exactly one thing. The index is what holds it; this proves the store answers
// in the caller's vocabulary rather than letting a 23505 surface as a 500.
func TestACriterionKeyIsUniqueWithinItsStage(t *testing.T) {
	e := setupConfigEnv(t)
	stageID := openStage(t, e, SemanticOpen)
	addCriterion(t, e, stageID, "buyer_confirmed")

	_, err := e.store.CreateStageExitCriterion(e.as(), CreateCriterionInput{
		StageID: stageID, Key: "buyer_confirmed", Label: "A second one",
		Kind: string(CriterionCustom),
	})
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != codeCriterionKeyTaken {
		t.Fatalf("a duplicate key answered %v, not the %s refusal a client can show", err, codeCriterionKeyTaken)
	}

	// A DIFFERENT stage may hold the same key: the criteria belong to the
	// stage, and two stages asking for the same thing is ordinary.
	other := openStage(t, e, SemanticOpen)
	addCriterion(t, e, other, "buyer_confirmed")
}

// Won and lost are where a deal stops, so there is nothing it takes to leave
// them. No CHECK can see the stage row from the criterion, so the store owns
// this and this test is what holds it.
func TestATerminalStageCarriesNoExitCriteria(t *testing.T) {
	e := setupConfigEnv(t)
	for _, semantic := range []StageSemantic{SemanticWon, SemanticLost} {
		stageID := openStage(t, e, semantic)
		_, err := e.store.CreateStageExitCriterion(e.as(), CreateCriterionInput{
			StageID: stageID, Key: "buyer_confirmed", Label: "Buyer confirmed",
			Kind: string(CriterionBuyerConfirmed),
		})
		var parse *values.ParseError
		if !errors.As(err, &parse) || parse.Code != codeTerminalStageNoCriteria {
			t.Errorf("a %s stage accepted a criterion (%v); it describes an exit that never happens", semantic, err)
		}
	}
}

// Archiving frees the key and keeps the row. Evidence cites a criterion by
// id, so a reader opening an older deal must still see what the stage asked
// for at the time — deleting it would make that history unreadable.
func TestArchivingACriterionKeepsItReadableAndFreesItsKey(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()
	stageID := openStage(t, e, SemanticOpen)
	first := addCriterion(t, e, stageID, "buyer_confirmed")

	if err := e.store.ArchiveStageExitCriterion(ctx, stageID,
		ids.From[ids.ExitCriterionKind](ids.UUID(first.Id)), nil); err != nil {
		t.Fatalf("archiving the criterion: %v", err)
	}

	// The key is free again, and taking it does not disturb the archived row.
	second := addCriterion(t, e, stageID, "buyer_confirmed")
	if second.Id == first.Id {
		t.Fatal("the re-added criterion reused the archived row rather than being its own fact")
	}

	live, err := e.store.ListStageExitCriteria(ctx, stageID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("listing live criteria: %v", err)
	}
	if len(live) != 1 || live[0].Id != second.Id {
		t.Fatalf("the live list holds %d criteria; the archived one is still being served", len(live))
	}

	all, err := e.store.ListStageExitCriteria(ctx, stageID, storekit.IncludeArchived)
	if err != nil {
		t.Fatalf("listing every criterion: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("the archived criterion is gone from the full list (%d rows); its evidence is now unreadable", len(all))
	}
}

// The survivors close the gap, so the editor never draws a hole and a later
// insert cannot collide with a position the archive left behind.
func TestArchivingACriterionClosesTheGapItLeft(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()
	stageID := openStage(t, e, SemanticOpen)
	first := addCriterion(t, e, stageID, "one")
	addCriterion(t, e, stageID, "two")
	addCriterion(t, e, stageID, "three")

	if err := e.store.ArchiveStageExitCriterion(ctx, stageID,
		ids.From[ids.ExitCriterionKind](ids.UUID(first.Id)), nil); err != nil {
		t.Fatalf("archiving the first criterion: %v", err)
	}
	live, err := e.store.ListStageExitCriteria(ctx, stageID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("listing live criteria: %v", err)
	}
	for want, c := range live {
		if c.Position != want {
			t.Errorf("criterion %s sits at position %d, not %d; the list has a hole in it", c.Key, c.Position, want)
		}
	}
}

// Editing pipeline shape needs pipeline:update, like every other stage edit.
// A reader who may only look is refused, and told so rather than shown a 404
// that hides whether the stage exists.
func TestOnlyAPipelineAdminEditsExitCriteria(t *testing.T) {
	e := setupConfigEnv(t)
	stageID := openStage(t, e, SemanticOpen)
	criterion := addCriterion(t, e, stageID, "buyer_confirmed")

	reader := principal.WithWorkspaceID(context.Background(), e.ws)
	reader = principal.WithCorrelationID(reader, ids.NewV7())
	reader = principal.WithActor(reader, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"member"},
			Objects:  map[string]principal.ObjectGrant{"pipeline": {Read: true}},
		},
	})

	if _, err := e.store.CreateStageExitCriterion(reader, CreateCriterionInput{
		StageID: stageID, Key: "sneaked_in", Label: "Sneaked in",
		Kind: string(CriterionCustom),
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader without pipeline:update added a criterion (%v)", err)
	}
	criterionID := ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id))
	label := "Renamed by a reader"
	if _, err := e.store.UpdateStageExitCriterion(reader, stageID, criterionID,
		UpdateCriterionInput{Label: &label}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader without pipeline:update edited a criterion (%v)", err)
	}
	if err := e.store.ArchiveStageExitCriterion(reader, stageID, criterionID, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader without pipeline:update archived a criterion (%v)", err)
	}

	// The positive control: the same reader may LIST, so the refusals above
	// are about the write and not about a fixture that denies everyone.
	if _, err := e.store.ListStageExitCriteria(reader, stageID, storekit.LiveOnly); err != nil {
		t.Errorf("a reader with pipeline:read could not list the criteria: %v", err)
	}
}

// Every criterion write is a fact the trail carries and the bus announces, so
// a subscriber watching stage shape sees the criteria change like any other.
func TestCreatingACriterionAuditsAndAnnouncesTheStage(t *testing.T) {
	e := setupConfigEnv(t)
	stageID := openStage(t, e, SemanticOpen)
	criterion := addCriterion(t, e, stageID, "buyer_confirmed")

	var auditRows int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'stage_exit_criterion' AND entity_id = $1 AND action = 'create'`,
		ids.UUID(criterion.Id)).Scan(&auditRows); err != nil {
		t.Fatalf("reading the audit trail: %v", err)
	}
	if auditRows != 1 {
		t.Errorf("the criterion wrote %d create audit rows; the trail cannot say who added it", auditRows)
	}

	var events int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'stage.updated'
		   AND envelope->'entity'->>'id' = $1::text`,
		stageID.String()).Scan(&events); err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	if events != 1 {
		t.Errorf("adding a criterion published %d stage.updated events; a subscriber cannot see the shape change", events)
	}
}

// The version an If-Match compares against has to MOVE, or the precondition
// is decorative: two editors both send If-Match: 1 and the second silently
// overwrites the first. stage and pipeline carry the same trigger.
func TestEditingACriterionAdvancesTheVersionItIsPinnedOn(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()
	stageID := openStage(t, e, SemanticOpen)
	criterion := addCriterion(t, e, stageID, "buyer_confirmed")
	criterionID := ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id))

	first := "Buyer said yes in writing"
	edited, err := e.store.UpdateStageExitCriterion(ctx, stageID, criterionID,
		UpdateCriterionInput{Label: &first, IfVersion: criterion.Version})
	if err != nil {
		t.Fatalf("the first edit was refused: %v", err)
	}
	if criterionVersion(edited) == criterionVersion(criterion) {
		t.Fatalf("the version stayed at %d through an edit; every If-Match would pass forever",
			criterionVersion(edited))
	}

	// The second editor still holds the ORIGINAL version, and must be refused.
	second := "Something else entirely"
	_, err = e.store.UpdateStageExitCriterion(ctx, stageID, criterionID,
		UpdateCriterionInput{Label: &second, IfVersion: criterion.Version})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("a stale If-Match was accepted (%v); the first editor's save is gone", err)
	}
}

// The terminal rule holds from both sides. refuseCriterionOnTerminalStage
// stops a criterion reaching a closed stage; this stops the stage closing
// while it still carries criteria, which is the same invariant and the one
// that made the contract's empty-list promise false.
func TestAStageCannotCloseWhileItStillAsksForCriteria(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()
	stageID := openStage(t, e, SemanticOpen)
	criterion := addCriterion(t, e, stageID, "buyer_confirmed")

	won := string(SemanticWon)
	_, err := e.store.UpdateStage(ctx, stageID, UpdateStageInput{Semantic: &won})
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != codeTerminalStageNoCriteria {
		t.Fatalf("an open stage carrying criteria closed as won (%v); the list now answers what the contract says is empty", err)
	}

	// Archiving the criteria is the way forward, and it works.
	if err := e.store.ArchiveStageExitCriterion(ctx, stageID,
		ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id)), nil); err != nil {
		t.Fatalf("archiving the criterion: %v", err)
	}
	if _, err := e.store.UpdateStage(ctx, stageID, UpdateStageInput{Semantic: &won}); err != nil {
		t.Fatalf("the stage still refused to close after its criteria were archived: %v", err)
	}
}

// Archiving a stage keeps its criteria READABLE — that is the whole reason
// they are archived rather than deleted, because evidence cites them.
func TestAnArchivedStagesCriteriaAreStillReadable(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()
	stageID := openStage(t, e, SemanticOpen)
	addCriterion(t, e, stageID, "buyer_confirmed")

	// Removing a stage is pipeline:delete, a different verb from the config
	// grant the rest of these cases use.
	remover := principal.WithWorkspaceID(context.Background(), e.ws)
	remover = principal.WithCorrelationID(remover, ids.NewV7())
	remover = principal.WithActor(remover, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"pipeline": {Create: true, Read: true, Update: true, Delete: true},
			},
		},
	})
	if err := e.store.ArchiveStage(remover, stageID, nil); err != nil {
		t.Fatalf("archiving the stage: %v", err)
	}
	criteria, err := e.store.ListStageExitCriteria(ctx, stageID, storekit.IncludeArchived)
	if err != nil {
		t.Fatalf("an archived stage's criteria became unreadable: %v", err)
	}
	if len(criteria) != 1 {
		t.Fatalf("the archived stage answered %d criteria; the evidence citing them cannot be read back", len(criteria))
	}
}
