// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A repeated search is planned with its words every time. After five runs of
// one prepared statement Postgres may switch to a generic plan, which keeps the
// query text a parameter and rebuilds every tsquery for each row it touches.
//
// The harness pool runs describe_exec, which never names a statement and so
// never reaches a generic plan; this test opens the product's own pool config
// on ONE connection (OwnPoolFromConfig keeps its exec mode), so every run
// reuses the statement cmd/api would reuse.
func TestARepeatedSearchIsNeverGenericallyPlanned(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	admin := e.Admin()
	if _, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{
		FullName: "Henrike Vossberg", Source: "manual",
	}); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}

	cfg, err := database.PoolConfig(os.Getenv("MARGINCE_TEST_APP_DSN"))
	if err != nil {
		t.Fatalf("configuring the product pool: %v", err)
	}
	cfg.MaxConns = 1
	pool, err := testdb.OwnPoolFromConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the product pool: %v", err)
	}
	t.Cleanup(pool.Close)

	store := search.NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.WS)))
	const runs = 8
	for range runs {
		page, err := store.Search(admin, search.Input{Query: "vossberg", Types: []string{"contact"}})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(page.Hits) != 1 {
			t.Fatalf("search found %d hits, want the one seeded contact", len(page.Hits))
		}
	}

	// This probe is itself a prepared statement carrying the marker, so it
	// excludes itself; exactly one other row must remain.
	rows, err := pool.Query(ctx, `
		SELECT generic_plans, custom_plans FROM pg_prepared_statements
		 WHERE statement LIKE '%AS rtype%' AND statement NOT LIKE '%pg_prepared_statements%'`)
	if err != nil {
		t.Fatalf("reading the ranking statement's plan counts: %v", err)
	}
	counts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Generic, Custom int64 }])
	if err != nil {
		t.Fatalf("reading the ranking statement's plan counts: %v", err)
	}
	if len(counts) != 1 {
		t.Fatalf("found %d cached ranking statements on the connection, want exactly one", len(counts))
	}
	if generic, custom := counts[0].Generic, counts[0].Custom; generic != 0 || custom != runs {
		t.Fatalf("the ranking statement ran %d generic and %d custom plans over %d searches; "+
			"every run must be custom-planned, or each row rebuilds the tsquery", generic, custom, runs)
	}
}
