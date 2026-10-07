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

// AssertIndexed fails t unless the quick-find over table, matching nameExpr and
// the identifier arm id (zero when the list has none), reads index and scans no
// table. Sequential scans are priced out for the one statement, so a scan left
// in the plan means the planner had no index to use, not that it chose not to
// on a small table.
func AssertIndexed(t *testing.T, table, nameExpr, index string, id storekit.Identifier) {
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
	used := false
	plans[0].Plan.walk(func(n planNode) {
		used = used || n.IndexName == index
		if n.NodeType == "Seq Scan" {
			t.Errorf("the %s quick-find scans %s, so every search reads that whole table. Plan: %s",
				table, n.RelationName, raw)
		}
	})
	if !used {
		t.Errorf("the %s quick-find over %s does not read %s; index the exact expression the query "+
			"matches. Plan: %s", table, nameExpr, index, raw)
	}
}
