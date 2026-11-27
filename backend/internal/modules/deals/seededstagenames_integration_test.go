// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/labeltranslation"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A new pipeline's closing pair reads in every shipped language, and renaming
// one shows the admin's name in every language. The same words an admin typed
// on a stage of their own survive a rename.
func TestANewPipelinesClosingStagesAreTranslatedUntilRenamed(t *testing.T) {
	t.Parallel()
	e := setupConfigEnv(t)
	ctx := e.as()
	pipeline, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: "Sales " + ids.NewV7().String()})
	if err != nil {
		t.Fatal(err)
	}
	var won ids.StageID
	for _, st := range *pipeline.Stages {
		if st.Name == defaultWonStageName {
			won = ids.From[ids.StageKind](ids.UUID(st.Id))
		}
		if got := labeltranslation.FromWire(st.NameTranslations); !maps.Equal(got, seededStageNames[st.Name]) {
			t.Errorf("stage %q has translations %v, want %v", st.Name, got, seededStageNames[st.Name])
		}
	}
	signed := "Signed"
	if after := e.renameStage(t, won, signed); len(after) != 0 {
		t.Errorf("a renamed seeded stage kept the seeded translations %v", after)
	}

	typed := e.createStage(t, CreateStageInput{
		PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), Name: "Proposal", Position: 1,
		NameTranslations: seededTranslations("Proposal"),
	})
	if after := e.renameStage(t, typed, "Quote"); !maps.Equal(after, seededStageNames["Proposal"]) {
		t.Errorf("a rename dropped the translations an admin typed: %v", after)
	}
}

// An older workspace's seeded stages gain their translations. A stage an admin
// added, renamed away and back, or translated is untouched, as is a stage of a
// pipeline an admin created. A second run changes nothing.
func TestTheRepairTranslatesOnlyTheStagesTheProductSeeded(t *testing.T) {
	t.Parallel()
	e := setupConfigEnv(t)
	olderSeed, err := e.store.CreatePipeline(e.system(), CreatePipelineInput{Name: "Sales " + ids.NewV7().String(), Stages: []StageInput{
		{Name: "Qualified", Position: 1, Semantic: string(DealOpen), WinProbability: 10},
		{Name: "Proposal", Position: 2, Semantic: string(DealOpen), WinProbability: 50},
		{Name: "Negotiation", Position: 3, Semantic: string(DealOpen), WinProbability: 75},
	}})
	if err != nil {
		t.Fatal(err)
	}
	stage := map[string]ids.StageID{}
	for _, st := range *olderSeed.Stages {
		stage[st.Name] = ids.From[ids.StageKind](ids.UUID(st.Id))
	}
	admin := map[string]string{"de": "Qualifiziert (Team)"}
	if _, err := e.store.UpdateStage(e.as(), stage["Qualified"], UpdateStageInput{NameTranslations: admin}); err != nil {
		t.Fatal(err)
	}
	e.renameStage(t, stage["Negotiation"], "Talks")
	e.renameStage(t, stage["Negotiation"], "Negotiation")
	added := e.createStage(t, CreateStageInput{PipelineID: ids.From[ids.PipelineKind](ids.UUID(olderSeed.Id)), Name: "Discovery", Position: 4})
	adminPipeline, err := e.store.CreatePipeline(e.as(), CreatePipelineInput{Name: "Partners " + ids.NewV7().String(), Stages: []StageInput{
		{Name: "Proposal", Position: 1, Semantic: string(DealOpen), WinProbability: 50},
	}})
	if err != nil {
		t.Fatal(err)
	}
	theirs := ids.From[ids.StageKind](ids.UUID((*adminPipeline.Stages)[0].Id))

	if changed := e.repairSeededStages(t); changed == 0 {
		t.Fatal("the repair changed no stage")
	}
	if changed := e.repairSeededStages(t); changed != 0 {
		t.Fatalf("a second repair changed %d stages, want none", changed)
	}
	for id, want := range map[ids.StageID]map[string]string{
		stage["Proposal"]: seededStageNames["Proposal"], stage["Qualified"]: admin,
		stage["Negotiation"]: {}, added: {}, theirs: {},
	} {
		if got := e.stageTranslations(t, id); !maps.Equal(got, want) {
			t.Errorf("stage %s has translations %v, want %v", id, got, want)
		}
	}
	if announced := e.repairEvents(t, stage["Proposal"]); announced != 1 {
		t.Errorf("the repair staged %d stage.updated events tied to its audit row, want 1", announced)
	}
}

func (e *configEnv) system() context.Context {
	return principal.SystemActing(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7()), "system:test")
}

func (e *configEnv) renameStage(t *testing.T, id ids.StageID, name string) map[string]string {
	t.Helper()
	st, err := e.store.UpdateStage(e.as(), id, UpdateStageInput{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	return labeltranslation.FromWire(st.NameTranslations)
}

// repairEvents counts the stage.updated events whose audit row is the
// repair's own write on the stage.
func (e *configEnv) repairEvents(t *testing.T, id ids.StageID) int {
	t.Helper()
	var announced int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM event_outbox o
		  JOIN audit_log a ON a.id::text = o.envelope->'trace'->>'audit_log_id'
		 WHERE o.envelope->>'type' = 'stage.updated' AND o.envelope->'entity'->>'id' = $1
		   AND a.entity_type = 'stage' AND a.entity_id = $2 AND a.after ? $3`,
		id.String(), id.UUID, seededTranslationsMark).Scan(&announced); err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	return announced
}

func (e *configEnv) createStage(t *testing.T, in CreateStageInput) ids.StageID {
	t.Helper()
	st, err := e.store.CreateStage(e.as(), in)
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.StageKind](ids.UUID(st.Id))
}

func (e *configEnv) repairSeededStages(t *testing.T) int {
	t.Helper()
	ctx := e.system()
	total := 0
	var after ids.UUID
	for {
		var scanned []ids.UUID
		if err := e.store.Tx(ctx, func(tx pgx.Tx) error {
			var changed int
			var err error
			scanned, changed, err = TranslateSeededStagesTx(ctx, tx, after, 50)
			total += changed
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if len(scanned) == 0 {
			return total
		}
		after = scanned[len(scanned)-1]
	}
}

func (e *configEnv) stageTranslations(t *testing.T, id ids.StageID) map[string]string {
	t.Helper()
	var got map[string]string
	if err := e.owner.QueryRow(context.Background(), `SELECT name_translations FROM stage WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	return got
}
