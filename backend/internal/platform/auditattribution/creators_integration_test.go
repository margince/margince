// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package auditattribution

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// planNode is the part of an EXPLAIN (ANALYZE, FORMAT JSON) node these tests read.
type planNode struct {
	RelationName    string     `json:"Relation Name"`          //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	ActualRows      float64    `json:"Actual Rows"`            //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	ActualLoops     float64    `json:"Actual Loops"`           //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	RemovedByFilter float64    `json:"Rows Removed by Filter"` //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	Plans           []planNode `json:"Plans"`                  //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
}

func (n planNode) walk(visit func(planNode)) {
	visit(n)
	for _, child := range n.Plans {
		child.walk(visit)
	}
}

// creatorsTx opens a transaction on the test database that is rolled back when
// the test ends, so the audit rows it seeds never outlive it.
func creatorsTx(t *testing.T) pgx.Tx {
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
		if err := conn.Close(ctx); err != nil {
			t.Errorf("closing the test connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, conn); err != nil {
		t.Fatalf("bringing the test database to head: %v", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the test transaction: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded audit rows: %v", err)
		}
	})
	return tx
}

func seedAudit(t *testing.T, tx pgx.Tx, id ids.UUID, action string, entity ids.UUID, actorID string, at time.Time) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, occurred_at)
		VALUES ($1, 'system', $2, $3, 'contact', $4, $5)`, id, actorID, action, entity, at); err != nil {
		t.Fatalf("seeding a %s row: %v", action, err)
	}
}

func TestCreatorsNameTheEarliestCreateRowOncePerRecord(t *testing.T) {
	tx := creatorsTx(t)
	at := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	replayed, tied, uncreated := ids.NewV7(), ids.NewV7(), ids.NewV7()
	seedAudit(t, tx, ids.NewV7(), "create", replayed, "system:replay", at.Add(time.Hour))
	seedAudit(t, tx, ids.NewV7(), "create", replayed, "system:first", at)
	seedAudit(t, tx, ids.NewV7(), "update", replayed, "system:earlier-update", at.Add(-time.Hour))
	smaller, larger := ids.NewV7(), ids.NewV7()
	seedAudit(t, tx, larger, "create", tied, "system:larger-id", at)
	seedAudit(t, tx, smaller, "create", tied, "system:smaller-id", at)
	seedAudit(t, tx, ids.NewV7(), "update", uncreated, "system:editor", at)

	got, err := CreatorsTx(context.Background(), tx, "contact", []ids.UUID{replayed, tied, uncreated, replayed})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d creators for two created records named three times: %+v", len(got), got)
	}
	if who := got[replayed].ActorID; who != "system:first" {
		t.Errorf("a replayed create names %q, want the earliest create row's system:first", who)
	}
	if who := got[tied].ActorID; who != "system:smaller-id" {
		t.Errorf("two creates at one instant name %q, want the smaller id's system:smaller-id", who)
	}
	if who, found := got[uncreated]; found {
		t.Errorf("a record with no create row names %+v, want it absent", who)
	}
}

// A page names its creators on every read, so the work per record must stay
// flat however long that record's history grows.
func TestCreatorsReadAFewAuditRowsPerRecordWhateverItsHistory(t *testing.T) {
	const records, updatesEach, rowsPerRecord = 200, 2000, 4
	tx := creatorsTx(t)
	ctx := context.Background()
	var page []ids.UUID
	if err := tx.QueryRow(ctx, `SELECT array_agg(uuidv7()) FROM generate_series(1, $1)`, records).Scan(&page); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, occurred_at)
		SELECT 'system', 'system:seed', CASE WHEN g = 0 THEN 'create' ELSE 'update' END, 'contact', record,
		       timestamptz '2026-01-01' + g * interval '1 minute'
		  FROM unnest($1::uuid[]) AS record CROSS JOIN generate_series(0, $2) g`, page, updatesEach); err != nil {
		t.Fatalf("seeding the record histories: %v", err)
	}
	if _, err := tx.Exec(ctx, `ANALYZE audit_log`); err != nil {
		t.Fatal(err)
	}

	var raw string
	if err := tx.QueryRow(ctx, `EXPLAIN (ANALYZE, FORMAT JSON) `+creatorsSQL, "contact", page).Scan(&raw); err != nil {
		t.Fatalf("explaining the creators statement: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"` //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	}
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan (%v): %s", err, raw)
	}
	auditNodes, read := 0, 0.0
	plans[0].Plan.walk(func(n planNode) {
		if n.RelationName == "audit_log" {
			auditNodes++
			read += (n.ActualRows + n.RemovedByFilter) * n.ActualLoops
		}
	})
	if auditNodes == 0 {
		t.Fatalf("the plan reads no audit_log node, so this test checks nothing: %s", raw)
	}
	if perRecord := read / records; perRecord > rowsPerRecord {
		t.Errorf("the creators statement reads %.0f audit rows per record of %d updates, want at most %d: %s",
			perRecord, updatesEach, rowsPerRecord, raw)
	}
}
