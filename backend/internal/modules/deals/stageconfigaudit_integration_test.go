// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// What a pipeline or stage edit says it changed FROM, against a real database.
//
// The claim is about a jsonb column two transactions apart: audit_log.before has
// to hold the value the row held BEFORE the update, and neither image may name a
// column the caller never sent. Nothing short of the real writer can show that —
// a fabricated row would prove only that the test can write jsonb — so every
// record here is seeded through CreatePipeline/CreateStage and changed through
// UpdatePipeline/UpdateStage.
//
// The win_probability case is the one a Go-side assertion cannot stand in for:
// a terminal semantic forces the committed value, so the test compares the
// after-image against the stage row itself rather than against the input.

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/platform/testdb"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

type configEnv struct {
	store *Store
	pool  *pgxpool.Pool
	owner *pgx.Conn
	ws    ids.UUID
	admin ids.UUID
}

func setupConfigEnv(t *testing.T) *configEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	// Before any seed: EnsureSchema rebuilds whenever it cannot prove the
	// database is a fresh lane clone, and a row written first would be dropped.
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}

	e := &configEnv{owner: owner, ws: ids.NewV7(), admin: ids.NewV7()}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		e.admin, "admin-"+e.admin.String()+"@config.test"); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.pool = pool
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)), Installation{})
	return e
}

// as is the pipeline administrator: the config surface gates on
// pipeline.create/read/update and nothing else.
func (e *configEnv) as() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects:  map[string]principal.ObjectGrant{"pipeline": {Create: true, Read: true, Update: true}},
		},
	})
}

// auditImages reads the images of the single `update` row written for one
// entity. A second row would mean the write under test audited twice, which the
// caller could not tell from the one it meant to read, so this refuses rather
// than picking.
func (e *configEnv) auditImages(t *testing.T, entityType string, entityID ids.UUID) (before, after map[string]any) {
	t.Helper()
	rows, err := e.owner.Query(context.Background(),
		`SELECT before, after FROM audit_log
		 WHERE entity_type = $1 AND entity_id = $2 AND action = 'update'`, entityType, entityID)
	if err != nil {
		t.Fatalf("reading the audit trail for %s %s: %v", entityType, entityID, err)
	}
	defer rows.Close()
	var images [][]byte
	for rows.Next() {
		var beforeJSON, afterJSON []byte
		if err := rows.Scan(&beforeJSON, &afterJSON); err != nil {
			t.Fatalf("scanning the audit trail: %v", err)
		}
		images = append(images, beforeJSON, afterJSON)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the audit trail: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("the %s update wrote %d audit rows, so there is no single before-image to judge", entityType, len(images)/2)
	}
	return decodeImage(t, "before", images[0]), decodeImage(t, "after", images[1])
}

func decodeImage(t *testing.T, which string, raw []byte) map[string]any {
	t.Helper()
	if raw == nil {
		t.Fatalf("the audit row's %s image is SQL NULL — the update cannot say what it changed %s", which, which)
	}
	var image map[string]any
	if err := json.Unmarshal(raw, &image); err != nil {
		t.Fatalf("the audit row's %s image is not an object: %v", which, err)
	}
	return image
}

// requireAbsent is the untouched-column half of the invariant: a field the
// caller never sent must not be reported as having changed, in either
// direction.
func requireAbsent(t *testing.T, before, after map[string]any, columns ...string) {
	t.Helper()
	for _, column := range columns {
		if value, present := before[column]; present {
			t.Fatalf("the before-image claims %s held %v, but this update never touched it", column, value)
		}
		if value, present := after[column]; present {
			t.Fatalf("the after-image claims %s became %v, but this update never touched it", column, value)
		}
	}
}

func TestARenamedPipelineRecordsTheNameItHadBefore(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()

	created, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: "Original", Position: 3})
	if err != nil {
		t.Fatalf("seeding the pipeline: %v", err)
	}
	id := ids.From[ids.PipelineKind](ids.UUID(created.Id))

	renamed := "Renamed"
	if _, err := e.store.UpdatePipeline(ctx, id, UpdatePipelineInput{Name: &renamed}); err != nil {
		t.Fatalf("renaming the pipeline: %v", err)
	}

	before, after := e.auditImages(t, "pipeline", id.UUID)
	if before["name"] != "Original" {
		t.Fatalf("the rename recorded %v as the previous name, but the row held \"Original\"", before["name"])
	}
	if after["name"] != renamed {
		t.Fatalf("the rename recorded %v as the new name, but it wrote %q", after["name"], renamed)
	}
	requireAbsent(t, before, after, "is_default", "position")
}

func TestARenamedStageRecordsTheNameItHadBefore(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()

	stage := e.seedStage(t, ctx, "Discovery", 20)
	renamed := "Qualified"
	if _, err := e.store.UpdateStage(ctx, stage, UpdateStageInput{Name: &renamed}); err != nil {
		t.Fatalf("renaming the stage: %v", err)
	}

	before, after := e.auditImages(t, "stage", stage.UUID)
	if before["name"] != "Discovery" {
		t.Fatalf("the rename recorded %v as the previous name, but the row held \"Discovery\"", before["name"])
	}
	if after["name"] != renamed {
		t.Fatalf("the rename recorded %v as the new name, but it wrote %q", after["name"], renamed)
	}
	requireAbsent(t, before, after, "position", stageSemanticField, "win_probability")
}

// A stage closed as won takes the probability the CHECK forces, not the one the
// caller sent. The after-image is judged against the ROW, because an image that
// merely echoed the input would be a record of something that never happened.
func TestAWonStageRecordsTheProbabilityTheRowActuallyHolds(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()

	stage := e.seedStage(t, ctx, "Negotiation", 60)
	won, ignored := string(SemanticWon), 42
	updated, err := e.store.UpdateStage(ctx, stage, UpdateStageInput{Semantic: &won, WinProbability: &ignored})
	if err != nil {
		t.Fatalf("closing the stage as won: %v", err)
	}

	before, after := e.auditImages(t, "stage", stage.UUID)
	if before["win_probability"] != float64(60) {
		t.Fatalf("the close recorded %v as the previous probability, but the row held 60", before["win_probability"])
	}
	if after["win_probability"] != float64(updated.WinProbability) {
		t.Fatalf("the close recorded %v as the committed probability while the row holds %d",
			after["win_probability"], updated.WinProbability)
	}
	if updated.WinProbability != 100 {
		t.Fatalf("a won stage holds %d, so the terminal rule stopped applying", updated.WinProbability)
	}
	requireAbsent(t, before, after, "name", "position")
}

// seedStage writes one open stage on a pipeline of its own, through the real
// creators, and hands back its id.
func (e *configEnv) seedStage(t *testing.T, ctx context.Context, name string, probability int) ids.StageID {
	t.Helper()
	pipeline, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: "Sales " + name})
	if err != nil {
		t.Fatalf("seeding the pipeline: %v", err)
	}
	stage, err := e.store.CreateStage(ctx, CreateStageInput{
		PipelineID:     ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)),
		Name:           name,
		Position:       1,
		Semantic:       string(SemanticOpen),
		WinProbability: &probability,
	})
	if err != nil {
		t.Fatalf("seeding the stage: %v", err)
	}
	return ids.From[ids.StageKind](ids.UUID(stage.Id))
}
