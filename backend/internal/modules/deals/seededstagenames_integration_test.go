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
// one shows the admin's name in every language.
func TestANewPipelinesClosingStagesAreTranslatedUntilRenamed(t *testing.T) {
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
	renamed := "Signed"
	after, err := e.store.UpdateStage(ctx, won, UpdateStageInput{Name: &renamed})
	if err != nil {
		t.Fatal(err)
	}
	if got := labeltranslation.FromWire(after.NameTranslations); len(got) != 0 {
		t.Errorf("a renamed seeded stage kept the seeded translations %v", got)
	}
}

// An older workspace's seeded stages gain their translations; a stage an admin
// renamed or translated is untouched, and a second run changes nothing.
func TestTheRepairTranslatesOnlyUntouchedSeededStages(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()
	pipeline, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: "Old " + ids.NewV7().String()})
	if err != nil {
		t.Fatal(err)
	}
	pipelineID := ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
	admin := map[string]string{"de": "Angebot raus"}
	seeded := e.createStage(t, CreateStageInput{PipelineID: pipelineID, Name: "Proposal", Position: 1})
	translated := e.createStage(t, CreateStageInput{PipelineID: pipelineID, Name: "Negotiation", Position: 2, NameTranslations: admin})
	renamed := e.createStage(t, CreateStageInput{PipelineID: pipelineID, Name: "Demo booked", Position: 3})

	if changed := e.repairSeededStages(t); changed == 0 {
		t.Fatal("the repair changed no stage")
	}
	if changed := e.repairSeededStages(t); changed != 0 {
		t.Fatalf("a second repair changed %d stages, want none", changed)
	}
	for id, want := range map[ids.StageID]map[string]string{
		seeded: seededStageNames["Proposal"], translated: admin, renamed: {},
	} {
		if got := e.stageTranslations(t, id); !maps.Equal(got, want) {
			t.Errorf("stage %s has translations %v, want %v", id, got, want)
		}
	}
	if _, after := e.auditImages(t, "stage", seeded.UUID); after[stageNameTranslationsField] == nil {
		t.Errorf("the repair did not audit the translations it wrote: %v", after)
	}
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
	ctx := principal.SystemActing(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7()), "system:test")
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
