// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// claimPlanNode is the part of an EXPLAIN (FORMAT JSON) node this test reads.
type claimPlanNode struct {
	NodeType  string          `json:"Node Type"`     //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	Relation  string          `json:"Relation Name"` //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	IndexCond string          `json:"Index Cond"`    //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	Plans     []claimPlanNode `json:"Plans"`         //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
}

func (n claimPlanNode) walk(visit func(claimPlanNode)) {
	visit(n)
	for _, child := range n.Plans {
		child.walk(visit)
	}
}

// The Worklist names the tasks on its page and asks for their recipients. The
// claims must be found by task, or every read walks every claim in the
// workspace and runs the visibility gates on each one.
//
// Sequential scans are off, so a small test table cannot make one look cheap.
// A claim lookup that can use an index on the task then does.
func TestTaskRecipientsAreFoundByTheirTask(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	// Claims spread over many contacts, as a workspace holds them: with one
	// contact the planner rightly starts from that contact instead.
	e.execAs(ctx, t, `
		WITH customers AS (
		  INSERT INTO contact (full_name, source, captured_by)
		  SELECT 'Customer ' || g, 'manual', 'human:test' FROM generate_series(1, 400) g RETURNING id),
		said AS (
		  INSERT INTO activity (kind, subject, source, captured_by, occurred_at)
		  SELECT 'email', 'said ' || g, 'manual', 'human:test', now() - (g || ' minutes')::interval
		    FROM generate_series(1, 4000) g RETURNING id),
		todo AS (
		  INSERT INTO activity (kind, subject, source, captured_by, occurred_at, assignee_id)
		  SELECT 'task', 'todo ' || g, 'manual', 'human:test', now(), $1::uuid
		    FROM generate_series(1, 4000) g RETURNING id)
		INSERT INTO conversation_claim
		  (contact_id, kind, body, source_activity_id, source_quote, evidence_fingerprint, source, captured_by, task_activity_id)
		SELECT customers.id, 'commitment_ours', 'send it', said.id, 'we will send it', said.id::text, 'manual', 'human:test', todo.id
		  FROM (SELECT id, row_number() OVER () AS n FROM said) said
		  JOIN (SELECT id, row_number() OVER () AS n FROM todo) todo USING (n)
		  JOIN (SELECT id, row_number() OVER () - 1 AS n FROM customers) customers ON customers.n = said.n % 400`, e.rep)
	analyzeAsOwner(t, "contact", "activity", "conversation_claim")
	var named []ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT task_activity_id FROM conversation_claim ORDER BY id LIMIT 3`)
		if err != nil {
			return err
		}
		named, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	}); err != nil {
		t.Fatalf("reading seeded tasks: %v", err)
	}

	query, args, err := taskClaimsStatement(ctx, named, "c.task_activity_id")
	if err != nil {
		t.Fatalf("building the statement: %v", err)
	}
	var raw []byte
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `EXPLAIN (FORMAT JSON) `+query, args...).Scan(&raw)
	}); err != nil {
		t.Fatalf("planning the recipient read: %v", err)
	}
	var plans []struct {
		Plan claimPlanNode `json:"Plan"` //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}
	byTask := false
	plans[0].Plan.walk(func(n claimPlanNode) {
		if n.Relation == "conversation_claim" && strings.Contains(n.IndexCond, "task_activity_id") {
			byTask = true
		}
	})
	if !byTask {
		t.Errorf("the claims are not looked up by their task:\n%s", raw)
	}
}

// analyzeAsOwner gives the planner the seeded tables' statistics, which only
// their owner may gather.
func analyzeAsOwner(t *testing.T, tables ...string) {
	t.Helper()
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatalf("connecting as the owner: %v", err)
	}
	defer func() {
		if err := owner.Close(ctx); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	}()
	if _, err := owner.Exec(ctx, "ANALYZE "+strings.Join(tables, ", ")); err != nil {
		t.Fatalf("analyzing %v: %v", tables, err)
	}
}
