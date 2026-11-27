// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package search

// The search branch over an extension kind, run against a real table. The
// trigram index serves its match, and a wildcard in the query matches itself.

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const planDemoTable = "ext_plandemo_thing"

// planDemoKind creates the kind's table with the index the readiness check
// requires, registers the kind, and answers a connection to the database.
func planDemoKind(t *testing.T) (*pgx.Conn, recordkinds.Declared) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testdb.For(t).OwnerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		recordkinds.Reset()
		if _, err := conn.Exec(context.Background(), `DROP TABLE IF EXISTS ext.`+planDemoTable); err != nil {
			t.Errorf("dropping the plan table: %v", err)
		}
		if err := conn.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := testdb.EnsureSchema(ctx, conn); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`DROP TABLE IF EXISTS ext.` + planDemoTable,
		`CREATE TABLE ext.` + planDemoTable + ` (id uuid PRIMARY KEY DEFAULT uuidv7(), archived_at timestamptz, code text NOT NULL UNIQUE)`,
		`CREATE INDEX ext_plandemo_thing_code_trgm ON ext.` + planDemoTable + ` USING gin (lower(code) public.gin_trgm_ops)`,
		`INSERT INTO ext.` + planDemoTable + ` (code) VALUES ('Spring 50% off'), ('Spring 50x off'), ('Autumn sale')`,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing the plan table (%s): %v", stmt, err)
		}
	}
	recordkinds.Register("plandemo", []extension.RecordKindDeclaration{{
		Kind: "thing", DisplayName: "Thing", Table: planDemoTable,
		Fields: []extension.RecordKindField{{Name: "code", Label: "Code"}}, NaturalKey: []string{"code"},
	}})
	kind, ok := recordkinds.Lookup("ext:plandemo:thing")
	if !ok {
		t.Fatal("the plan kind did not register")
	}
	return conn, kind
}

// extensionBranch is the one search branch over the kind for query.
func extensionBranch(t *testing.T, kind recordkinds.Declared, query string) (string, []any) {
	t.Helper()
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	head := arg(query)
	branches := admittedExtensionSQL(readerFor(kind.Object), []string{kind.Name}, query, head, 0, arg)
	if len(branches) != 1 {
		t.Fatalf("got %d branches for the kind, want 1", len(branches))
	}
	return branches[0], args
}

func TestAnExtensionKindSearchUsesItsTrigramIndex(t *testing.T) {
	t.Parallel()
	conn, kind := planDemoKind(t)
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("rolling back the plan transaction: %v", err)
		}
	})
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	branch, args := extensionBranch(t, kind, "spring sale")
	rows, err := tx.Query(ctx, "EXPLAIN "+branch, args...)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(plan, "\n")
	if !strings.Contains(text, "ext_plandemo_thing_code_trgm") || strings.Contains(text, "Seq Scan") {
		t.Fatalf("the search branch does not read the trigram index:\n%s", text)
	}
}

func TestAWildcardInAnExtensionKindSearchMatchesItself(t *testing.T) {
	t.Parallel()
	conn, kind := planDemoKind(t)
	branch, args := extensionBranch(t, kind, "50%")
	rows, err := conn.Query(context.Background(), "SELECT title FROM ("+branch+") hit", args...)
	if err != nil {
		t.Fatal(err)
	}
	titles, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(titles, []string{"Spring 50% off"}) {
		t.Errorf("a search for 50%% found %q, want only the code holding it", titles)
	}
}
