// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// An install that stored rows of an extension kind before the kind columns
// were narrowed upgrades without losing one. signal and user_record_view are
// not seeded: their entity shape CHECKs never let an extension kind be stored.
func TestNarrowingTheKindColumnsKeepsStoredExtensionRows(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1795639501")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 2); err != nil {
		t.Fatalf("rolling the narrowing back: %v", err)
	}

	const kind = "ext:visit-log:visit"
	const low, high = "0197a000-0000-7000-8000-000000000001", "0197a000-0000-7000-8000-000000000002"
	for _, seed := range []struct{ what, sql string }{
		{"attachment", `INSERT INTO attachment (entity_type, entity_id, filename, storage_key, source, captured_by)
			VALUES ($1, gen_random_uuid(), 'visit.pdf', 'k/visit.pdf', 'test', 'system')`},
		{"embedding", `INSERT INTO embedding (entity_type, entity_id, chunk_hash, model, embedding)
			VALUES ($1, gen_random_uuid(), 'h', 'm', '[1,2,3]'::vector)`},
		{"field_provenance", `INSERT INTO field_provenance (object_type, object_id, field_name, source, captured_by)
			VALUES ($1, gen_random_uuid(), 'name', 'test', 'system')`},
		{"dedupe_candidate", `INSERT INTO dedupe_candidate (entity_type, left_ext_id, right_ext_id, confidence, evidence, source, captured_by)
			VALUES ($1, '` + low + `', '` + high + `', 0.9, '{}', 'test', 'system')`},
	} {
		if _, err := conn.Exec(ctx, seed.sql, kind); err != nil {
			t.Fatalf("seeding %s with an extension kind: %v", seed.what, err)
		}
	}

	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("the upgrade stopped on stored extension rows: %v", err)
	}
	for _, c := range []struct{ table, column string }{
		{"attachment", "entity_type"},
		{"embedding", "entity_type"},
		{"field_provenance", "object_type"},
		{"dedupe_candidate", "entity_type"},
	} {
		var kept int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM `+c.table+` WHERE `+c.column+` = $1`, kind).Scan(&kept); err != nil {
			t.Fatal(err)
		}
		if kept != 1 {
			t.Errorf("%s keeps %d rows of the extension kind, want the 1 it had", c.table, kept)
		}
		var validated bool
		if err := conn.QueryRow(ctx, `SELECT convalidated FROM pg_constraint WHERE conname = $1`,
			c.table+"_"+c.column+"_check").Scan(&validated); err != nil {
			t.Fatal(err)
		}
		if validated {
			t.Errorf("%s's kind CHECK was validated over a row it refuses", c.table)
		}
	}
	var left, right string
	if err := conn.QueryRow(ctx, `SELECT left_ext_id::text, right_ext_id::text FROM dedupe_candidate WHERE entity_type = $1`, kind).
		Scan(&left, &right); err != nil {
		t.Fatalf("reading the stored extension pair: %v", err)
	}
	if left != low || right != high {
		t.Errorf("the extension pair is %s/%s, want %s/%s", left, right, low, high)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO field_provenance (object_type, object_id, field_name, source, captured_by)
		VALUES ($1, gen_random_uuid(), 'name', 'test', 'system')`, kind); err == nil {
		t.Error("a new row of an extension kind was accepted after the upgrade")
	}
}

// The narrowing runs again over its own result and leaves the same schema. A
// database migrated under an earlier number is in that state. The dedupe pair
// columns are gone by then, so the up file must not parse a statement naming them.
func TestNarrowingTheKindColumnsRunsAgainOverItsOwnResult(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	for _, file := range []string{
		"core/1795639500_core_only_kind_columns_refuse_an_extension_kind.up.sql",
		"core/1795639501_core_only_kind_columns_are_validated.up.sql",
	} {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			rollbackErr := tx.Rollback(ctx)
			t.Fatalf("running %s a second time: %v (rollback: %v)", file, err, rollbackErr)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var pairColumns, unvalidated int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = 'dedupe_candidate'
		   AND column_name IN ('left_ext_id', 'right_ext_id')`).Scan(&pairColumns); err != nil {
		t.Fatal(err)
	}
	if pairColumns != 0 {
		t.Errorf("dedupe_candidate has %d extension pair column(s) after the rerun", pairColumns)
	}
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint
		 WHERE conname = ANY ($1) AND NOT convalidated`, []string{
		"attachment_entity_type_check", "embedding_entity_type_check", "field_provenance_object_type_check",
		"user_record_view_entity_type_check", "signal_entity_type_check",
		"dedupe_candidate_entity_type_check", "dedupe_candidate_shape",
	}).Scan(&unvalidated); err != nil {
		t.Fatal(err)
	}
	if unvalidated != 0 {
		t.Errorf("%d narrowed CHECK(s) are NOT VALID after the rerun over a clean schema", unvalidated)
	}
}
