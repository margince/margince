// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package quickfindtest proves that a list's quick-find reads its trigram
// index. storekit.QuickFindClause matches a name expression the caller passes
// in, and the index is a separate statement in a migration; nothing else fails
// when the two drift apart, because the results stay right and only the plan
// turns into a scan of the whole table.
package quickfindtest

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

// The keys are Postgres's EXPLAIN JSON, not ours.
//
//nolint:tagliatelle // fixed by the server's plan format
type planNode struct {
	NodeType     string     `json:"Node Type"`
	RelationName string     `json:"Relation Name"`
	IndexName    string     `json:"Index Name"`
	Plans        []planNode `json:"Plans"`
}

// explained is one statement in EXPLAIN's JSON array.
//
//nolint:tagliatelle // fixed by the server's plan format
type explained struct {
	Plan planNode `json:"Plan"`
}

func (n planNode) walk(visit func(planNode)) {
	visit(n)
	for _, child := range n.Plans {
		child.walk(visit)
	}
}

// AssertIndexed fails t unless the quick-find over table reads index and scans
// no table. It matches nameExpr and the identifier arm id (zero when the list
// has none), and when id is set it also requires identifierIndex. Sequential scans are priced out
// for the one statement. A scan left in the plan therefore means the planner
// had no index to use.
func AssertIndexed(t *testing.T, table, nameExpr, index string, id storekit.Identifier, identifierIndex string) {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to the test database: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing the plan connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, conn); err != nil {
		t.Fatalf("bringing the test database to head: %v", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the plan transaction: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the plan transaction: %v", err)
		}
	}()
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("pricing out sequential scans: %v", err)
	}
	if id.Table != "" {
		if err := dropCompetingIndexes(ctx, tx, id.Table, identifierIndex, index); err != nil {
			t.Fatalf("clearing the other %s indexes for the plan: %v", id.Table, err)
		}
	}
	statement := `EXPLAIN (FORMAT JSON) SELECT id FROM ` + pgx.Identifier{table}.Sanitize() +
		` WHERE ` + storekit.QuickFindClauseWith(1, nameExpr, id)
	var raw []byte
	if err := tx.QueryRow(ctx, statement, "fragment").Scan(&raw); err != nil {
		t.Fatalf("explaining the %s quick-find: %v", table, err)
	}
	var plans []explained
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the %s quick-find plan (%v): %s", table, err, raw)
	}
	used, identifierUsed := false, false
	plans[0].Plan.walk(func(n planNode) {
		used = used || n.IndexName == index
		identifierUsed = identifierUsed || n.IndexName == identifierIndex
		if n.NodeType == "Seq Scan" {
			t.Errorf("the %s quick-find scans %s, so every search reads that whole table. Plan: %s",
				table, n.RelationName, raw)
		}
	})
	if !used {
		t.Errorf("the %s quick-find over %s does not read %s; index the exact expression the query "+
			"matches. Plan: %s", table, nameExpr, index, raw)
	}
	if id.Table != "" && !identifierUsed {
		t.Errorf("the %s identifier arm does not read %s; index the exact expression the contains "+
			"arm matches. Plan: %s", table, identifierIndex, raw)
	}
}

// dropCompetingIndexes drops, inside the plan transaction, every index on table
// except those in keep, the full-text index and those backing a constraint.
// On an empty test table the planner prices a trigram index above a partial
// btree that covers the same rows. Without the drop, the plan would never show
// whether the identifier index can serve the contains arm.
// A lead's own address shares its table with the name arm, hence keep.
func dropCompetingIndexes(ctx context.Context, tx pgx.Tx, table string, keep ...string) error {
	rows, err := tx.Query(ctx, `
		SELECT i.relname
		FROM pg_index x
		JOIN pg_class i ON i.oid = x.indexrelid
		WHERE x.indrelid = $1::regclass AND i.relname <> ALL ($2)
		  AND pg_get_indexdef(x.indexrelid) NOT LIKE '%search_tsv%'
		  AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = x.indexrelid)`,
		pgx.Identifier{table}.Sanitize(), keep)
	if err != nil {
		return err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, name := range names {
		if _, err := tx.Exec(ctx, `DROP INDEX `+pgx.Identifier{name}.Sanitize()); err != nil {
			return err
		}
	}
	return nil
}
