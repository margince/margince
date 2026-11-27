// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

const directionPrefixIndex, directionPageIndex = "idx_activity_direction", "idx_activity_direction_page"

func TestTheDirectionIndexReplacementKeepsARecordedIndexAcrossRetries(t *testing.T) {
	t.Parallel()
	owner, _ := dsns(t)
	conn := connect(t, owner)
	headSchema(t, conn)
	ctx := context.Background()
	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatal(err)
	}
	const build, retire = "1794721796", "1794722321"
	replacement := namespaceThroughVersion(t, core, retire)
	if _, err := dbmigrate.Down(ctx, conn, replacement, 2); err != nil {
		t.Fatal(err)
	}
	assertDirectionIndexes(t, conn, directionPrefixIndex)
	buildOnly := core
	buildOnly.Migrations = nil
	for _, migration := range core.Migrations {
		if migration.Version != retire {
			buildOnly.Migrations = append(buildOnly.Migrations, migration)
		}
	}
	for attempt := range 2 {
		if _, err := dbmigrate.Up(ctx, conn, buildOnly); err != nil {
			t.Fatal(err)
		}
		assertDirectionIndexes(t, conn, directionPrefixIndex, directionPageIndex)
		if attempt == 0 {
			forgetDirectionMigration(t, conn, build)
		}
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatal(err)
	}
	assertDirectionIndexes(t, conn, directionPageIndex)
	var before uint32
	if err := conn.QueryRow(ctx, "SELECT 'idx_activity_direction_page'::regclass::oid").Scan(&before); err != nil {
		t.Fatal(err)
	}
	forgetDirectionMigration(t, conn, retire)
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatal(err)
	}
	var after uint32
	if err := conn.QueryRow(ctx, "SELECT 'idx_activity_direction_page'::regclass::oid").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Error("retrying the retirement rebuilt the only direction index")
	}
	if _, err := dbmigrate.Down(ctx, conn, replacement, 2); err != nil {
		t.Fatal(err)
	}
	assertDirectionIndexes(t, conn, directionPrefixIndex)
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatal(err)
	}
	assertDirectionIndexes(t, conn, directionPageIndex)
}

func forgetDirectionMigration(t *testing.T, conn *pgx.Conn, version string) {
	t.Helper()
	result, err := conn.Exec(context.Background(), "DELETE FROM schema_migrations_core WHERE version = @version", pgx.NamedArgs{"version": version})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsAffected() != 1 {
		t.Fatalf("forgetting migration %s removed %d rows", version, result.RowsAffected())
	}
}

func assertDirectionIndexes(t *testing.T, conn *pgx.Conn, expected ...string) {
	t.Helper()
	for _, name := range []string{directionPrefixIndex, directionPageIndex} {
		want := slices.Contains(expected, name)
		var valid bool
		if err := conn.QueryRow(context.Background(), `SELECT EXISTS (
		 SELECT 1 FROM pg_index WHERE indexrelid = to_regclass(@name) AND indisvalid AND indisready)`, pgx.NamedArgs{"name": name}).Scan(&valid); err != nil {
			t.Fatal(err)
		}
		if valid != want {
			t.Errorf("index %s valid=%t, want %t", name, valid, want)
		}
	}
}
