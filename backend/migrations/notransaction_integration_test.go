// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A migration may ask not to be wrapped, and CREATE INDEX CONCURRENTLY is why.
//
// Postgres refuses a concurrent build inside a transaction block, and this runner wrapped
// every file — so the concurrent build was not a choice anyone declined, it was
// unexpressible. These tests prove both halves against a real server: the wrapped
// spelling is refused by Postgres itself, and the unwrapped one lands with its
// bookkeeping row.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

// concurrentIndexNamespace is one throwaway namespace whose second migration builds an
// index concurrently, asked for with the marker or not as the caller wants.
func concurrentIndexNamespace(t *testing.T, name string, ask bool) dbmigrate.Namespace {
	t.Helper()
	// Asked: the PAIR, which is the shape the gate requires and the one the runner has
	// to execute — the drop is what makes a retry safe.
	//
	// Not asked: the BUILD ALONE. With the drop first, Postgres refuses that statement
	// and the test would never reach the build it claims to be about.
	build := "CREATE INDEX CONCURRENTLY idx_unwrapped_probe ON unwrapped_probe (x);"
	if ask {
		build = dbmigrate.NoTransactionMarker +
			"\nDROP INDEX CONCURRENTLY IF EXISTS idx_unwrapped_probe;\n" + build
	}
	return dbmigrate.Namespace{
		Name: name,
		Migrations: []dbmigrate.Migration{
			{
				Version: "0001", Name: "table",
				UpSQL:   "CREATE TABLE unwrapped_probe (x uuid);",
				DownSQL: "DROP TABLE unwrapped_probe;",
			},
			{
				Version: "0002", Name: "index",
				UpSQL: build,
				DownSQL: dbmigrate.NoTransactionMarker +
					"\nDROP INDEX CONCURRENTLY IF EXISTS idx_unwrapped_probe;",
			},
		},
	}
}

// Wrapped, Postgres refuses it — which is the reason this mode exists, stated by the
// server rather than by a comment.
func TestAWrappedConcurrentIndexBuildIsRefusedByPostgres(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	_, err := dbmigrate.Up(ctx, conn, concurrentIndexNamespace(t, "probe_wrapped", false))
	if err == nil {
		t.Fatal("a concurrent index build succeeded inside a transaction, so this mode has no " +
			"reason to exist and the marker is dead weight")
	}
	if !strings.Contains(err.Error(), "CREATE INDEX CONCURRENTLY") {
		t.Errorf("the failure was %v, which is not Postgres refusing the concurrent build — the "+
			"test may be failing for an unrelated reason and proving nothing", err)
	}
}

// Unwrapped, it lands — and the ledger records it.
func TestAnUnwrappedConcurrentIndexBuildLandsAndIsRecorded(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	ns := concurrentIndexNamespace(t, "probe_unwrapped", true)
	applied, err := dbmigrate.Up(ctx, conn, ns)
	if err != nil {
		t.Fatalf("up → %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied %d migrations, want 2", applied)
	}

	var valid bool
	if err := conn.QueryRow(ctx, `
		SELECT i.indisvalid FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		 WHERE c.relname = 'idx_unwrapped_probe'`).Scan(&valid); err != nil {
		t.Fatalf("reading the index back: %v", err)
	}
	if !valid {
		t.Error("the index exists but is INVALID, which is what a failed concurrent build leaves " +
			"— the build did not finish")
	}

	// The bookkeeping row is the half that cannot share the work's transaction, so it
	// is the half worth asserting.
	var recorded int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM schema_migrations_probe_unwrapped WHERE version = '0002'`).
		Scan(&recorded); err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	if recorded != 1 {
		t.Errorf("the ledger holds %d rows for the unwrapped migration, want 1: work done and "+
			"unrecorded makes the next run apply it again", recorded)
	}

	// And a re-run applies nothing, which is what the ledger row is for.
	again, err := dbmigrate.Up(ctx, conn, ns)
	if err != nil {
		t.Fatalf("second up → %v", err)
	}
	if again != 0 {
		t.Errorf("the re-run applied %d, want 0", again)
	}

	// The case the ledger cannot cover: the work landed and the bookkeeping did not,
	// which a crash between the two statements leaves behind. Applying the same file
	// again has to succeed, because that is the whole contract of the mode — and it is
	// the drop above the build that makes it, since the index already exists by now.
	if _, err := conn.Exec(ctx,
		`DELETE FROM schema_migrations_probe_unwrapped WHERE version = '0002'`); err != nil {
		t.Fatalf("forgetting the ledger row: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, ns); err != nil {
		t.Fatalf("re-applying an unwrapped migration whose ledger row was lost → %v: a crash "+
			"between the DDL and the bookkeeping would leave this database unmigratable", err)
	}
}
