// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

const importRecordOriginChecksVersion = "20261013213808"

// seedIdentity writes one import_record_map row in the shape code before the
// origin column wrote.
func seedIdentity(t *testing.T, conn *pgx.Conn, runID, externalID string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO import_record_map (source_system, object, external_id, native_id, import_run_id)
		VALUES ('import:csv:origin', 'company', $1, gen_random_uuid(), $2::uuid)`, externalID, runID); err != nil {
		t.Fatalf("seeding the identity %s: %v", externalID, err)
	}
}

func originOf(t *testing.T, conn *pgx.Conn, externalID string) string {
	t.Helper()
	var origin string
	if err := conn.QueryRow(context.Background(), `
		SELECT origin FROM import_record_map
		 WHERE source_system = 'import:csv:origin' AND external_id = $1`, externalID).Scan(&origin); err != nil {
		t.Fatalf("reading the origin of %s: %v", externalID, err)
	}
	return origin
}

func TestAnOldShapeIdentityRowIsACreation(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	run := seedLegacyRun(t, conn, legacyRun{status: "complete", sourceRef: "ws/import/origin-old"})
	seedIdentity(t, conn, run, "CO-1")

	// Undo archives what a creation maps, so an old writer's row must read as one.
	if got := originOf(t, conn, "CO-1"); got != "created" {
		t.Errorf("an old-shape identity row has origin %q, want created", got)
	}
}

func TestTheOriginCheckRefusesAnUnknownOrigin(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	run := seedLegacyRun(t, conn, legacyRun{status: "complete", sourceRef: "ws/import/origin-check"})
	seedIdentity(t, conn, run, "CO-1")
	ctx := context.Background()

	if _, err := conn.Exec(ctx, `UPDATE import_record_map SET origin = 'bound' WHERE external_id = 'CO-1'`); err != nil {
		t.Fatalf("origin bound was refused: %v", err)
	}
	_, err := conn.Exec(ctx, `UPDATE import_record_map SET origin = 'adopted' WHERE external_id = 'CO-1'`)
	if err == nil || !strings.Contains(err.Error(), "import_record_map_origin_check") {
		t.Fatalf("origin 'adopted' = %v, want the origin check to refuse it", err)
	}
	var validated bool
	if err := conn.QueryRow(ctx,
		`SELECT convalidated FROM pg_constraint WHERE conname = 'import_record_map_origin_check'`).Scan(&validated); err != nil {
		t.Fatal(err)
	}
	if !validated {
		t.Error("the origin check is still NOT VALID at head; the second migration validates it")
	}
}

func TestTheOriginDownMigrationDropsBoundRowsAndKeepsCreatedOnes(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	run := seedLegacyRun(t, conn, legacyRun{status: "complete", sourceRef: "ws/import/origin-down"})
	seedIdentity(t, conn, run, "CO-created")
	seedIdentity(t, conn, run, "CO-bound")
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `UPDATE import_record_map SET origin = 'bound' WHERE external_id = 'CO-bound'`); err != nil {
		t.Fatal(err)
	}

	rewound := namespaceThroughVersion(t, customNamespace(t), importRecordOriginChecksVersion)
	if _, err := dbmigrate.Down(ctx, conn, rewound, 2); err != nil {
		t.Fatalf("down: %v", err)
	}
	// Code without the column reads every row as a creation. Its undo would
	// archive a bound record, so no bound row may survive the down.
	rows, err := conn.Query(ctx, `
		SELECT external_id FROM import_record_map WHERE source_system = 'import:csv:origin' ORDER BY external_id`)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || kept[0] != "CO-created" {
		t.Errorf("after down the map holds %v, want only CO-created", kept)
	}
	var columns int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'import_record_map' AND column_name = 'origin'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 0 {
		t.Error("after down import_record_map still has origin")
	}
	replayImportSession(t, conn)
}
