// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contact360

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The keys are Postgres's EXPLAIN JSON, not ours.
//
//nolint:tagliatelle // fixed by the server's plan format
type analyzedNode struct {
	SubplanName string         `json:"Subplan Name"`
	ActualRows  float64        `json:"Actual Rows"`
	Plans       []analyzedNode `json:"Plans"`
}

// gatedRows is how many rows the statement's one content gate judged.
func gatedRows(t *testing.T, raw string) float64 {
	t.Helper()
	var plans []struct {
		Plan analyzedNode `json:"Plan"` //nolint:tagliatelle // fixed by the server's plan format
	}
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}
	found, rows := false, 0.0
	var walk func(analyzedNode)
	walk = func(n analyzedNode) {
		if n.SubplanName == "CTE readable_activity" {
			found, rows = true, n.ActualRows
		}
		for _, child := range n.Plans {
			walk(child)
		}
	}
	walk(plans[0].Plan)
	if !found {
		t.Fatalf("the plan has no readable_activity CTE — the statement changed shape:\n%s", raw)
	}
	return rows
}

// A contact's last touch judges a bounded number of rows however long its
// history. The gate runs on each direction's newest window, never on every
// message the contact was ever on.
func TestALastTouchGatesAWindowNotTheWholeHistory(t *testing.T) {
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded history: %v", err)
		}
	}()
	contact := ids.From[ids.ContactKind](ids.NewV7())
	const history = 300
	// Years of mail both ways, every message the contact wrote or received.
	seeds := []struct {
		statement string
		args      []any
	}{
		{`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Long History', 'manual', 'human:seed')`, []any{contact}},
		{`CREATE TEMP TABLE seeded ON COMMIT DROP AS
		  SELECT gen_random_uuid() id, g, CASE WHEN g % 2 = 0 THEN 'inbound' ELSE 'outbound' END direction
		    FROM generate_series(1, $1::int) g`, []any{history}},
		{`INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		  SELECT id, 'email', 'Topic '||g, now() - g * interval '1 day', direction, 'manual', 'human:seed' FROM seeded`, nil},
		{`INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT id, 'contact', $1 FROM seeded`, []any{contact}},
		{`INSERT INTO activity_participant (activity_id, contact_id, role)
		  SELECT id, $1, CASE WHEN direction = 'inbound' THEN 'from' ELSE 'to' END FROM seeded`, []any{contact}},
	}
	for _, seed := range seeds {
		if _, err := tx.Exec(ctx, seed.statement, seed.args...); err != nil {
			t.Fatalf("seeding the history: %v", err)
		}
	}
	reader := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:reader", UserID: ids.NewV7(),
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true}, "contact": {Read: true},
		}},
	})
	judged := func(window int) float64 {
		query, args, err := lastTouchStatement(reader, []ids.ContactID{contact}, AssembleOptions{}, window)
		if err != nil {
			t.Fatalf("building the statement: %v", err)
		}
		var raw string
		if err := tx.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+query, args...).Scan(&raw); err != nil {
			t.Fatalf("running the statement: %v", err)
		}
		return gatedRows(t, raw)
	}
	// Each direction keeps its own window: the newest replies and the newest
	// sends, which the live and any-exchange windows fall inside.
	if got, want := judged(lastTouchWindow), float64(2*lastTouchWindow); got != want {
		t.Errorf("the windowed read gated %v rows of a %d-message history, want %v", got, history, want)
	}
	if got := judged(0); got < history {
		t.Errorf("the whole-history read gated %v rows, want every one of the %d messages: the census cannot tell the two apart", got, history)
	}
}
