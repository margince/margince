// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

//nolint:tagliatelle // PostgreSQL EXPLAIN defines these JSON keys.
type neighbourPlanNode struct {
	Relation    string              `json:"Relation Name"`
	Rows        int                 `json:"Actual Rows"`
	Filtered    int                 `json:"Rows Removed by Filter"`
	Loops       int                 `json:"Actual Loops"`
	SharedHits  int                 `json:"Shared Hit Blocks"`
	SharedReads int                 `json:"Shared Read Blocks"`
	Plans       []neighbourPlanNode `json:"Plans"`
}

//nolint:tagliatelle // PostgreSQL EXPLAIN defines these JSON keys.
type neighbourExplanation struct {
	Plan        neighbourPlanNode `json:"Plan"`
	ExecutionMS float64           `json:"Execution Time"`
}

func (p neighbourPlanNode) activityRows() int {
	rows := 0
	if p.Relation == "activity" {
		rows = (p.Rows + p.Filtered) * p.Loops
	}
	for _, child := range p.Plans {
		rows += child.activityRows()
	}
	return rows
}

type neighbourPlanTx struct {
	pgx.Tx
	plan []byte
}

func (p *neighbourPlanTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if err := p.Tx.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+sql, args...).Scan(&p.plan); err != nil {
		return nil, err
	}
	return p.Tx.Query(ctx, sql, args...)
}

func TestMailNeighboursSeekPastUnrelatedHistory(t *testing.T) {
	e := integration.SetupSearch(t)
	if _, err := e.Owner.Exec(e.Admin(), "ANALYZE activity,activity_identity,activity_mail_reference"); err != nil {
		t.Fatal(err)
	}
	var candidates []ids.ActivityID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.Admin(), `INSERT INTO activity (kind, source, source_system, source_id, thread_key, captured_by)
   SELECT 'email', 'manual', 'email', 'background-' || n, 'background-' || n, 'test:neighbours' FROM generate_series(1, 10000) n`)
		if err != nil {
			return err
		}
		keys := []string{"target@neighbour.test", "identity-source", "reverse-source", "self-source", "archived-source", "unthreaded-source"}
		candidates = make([]ids.ActivityID, 0, len(keys))
		for _, key := range keys {
			var id ids.ActivityID
			err := tx.QueryRow(e.Admin(), `INSERT INTO activity (kind, source, source_system, source_id, thread_key, captured_by)
    VALUES ('email', 'manual', 'email', $1, NULLIF($2, 'unthreaded-source'), 'test:neighbours') RETURNING id`, key, key).Scan(&id)
			if err != nil {
				return err
			}
			candidates = append(candidates, id)
		}
		if _, err := tx.Exec(e.Admin(), `INSERT INTO activity_identity (activity_id, identity_kind, identity_key, attested_by)
   VALUES ($1, 'mail', 'target@neighbour.test', 'test:neighbours')`, candidates[1]); err != nil {
			return err
		}
		if _, err := tx.Exec(e.Admin(), `INSERT INTO activity_mail_reference (activity_id, referenced_id)
   SELECT id, 'own@neighbour.test' FROM unnest($1::uuid[]) AS id`, candidates[:5]); err != nil {
			return err
		}
		if _, err := tx.Exec(e.Admin(), `UPDATE activity SET archived_at = '2020-01-01T00:00:00Z' WHERE id = $1`, candidates[4]); err != nil {
			return err
		}
		return checkNeighbourPlan(t, e, tx, candidates, "stale")
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Owner.Exec(e.Admin(), "ANALYZE activity,activity_identity,activity_mail_reference"); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error { return checkNeighbourPlan(t, e, tx, candidates, "fresh") }); err != nil {
		t.Fatal(err)
	}
}

func checkNeighbourPlan(t *testing.T, e *integration.SearchEnv, tx pgx.Tx, candidates []ids.ActivityID, statistics string) error {
	t.Helper()
	measured := &neighbourPlanTx{Tx: tx}
	got, err := activities.MailNeighboursTx(e.Admin(), measured, candidates[3], "own@neighbour.test", []string{"target@neighbour.test", "archived-source", "unthreaded-source"})
	if err != nil {
		return err
	}
	if len(got) != 3 {
		t.Fatalf("neighbours=%v want=%v", got, candidates[:3])
	}
	for _, wanted := range candidates[:3] {
		if !slices.Contains(got, wanted) {
			t.Fatalf("neighbours=%v missing=%v", got, wanted)
		}
	}

	var plans []neighbourExplanation
	if err := json.Unmarshal(measured.plan, &plans); err != nil {
		return err
	}
	if len(plans) != 1 {
		t.Fatalf("plans=%d want=1", len(plans))
	}

	if visited := plans[0].Plan.activityRows(); visited > 10 {
		t.Fatalf("neighbour lookup visited %d activity rows for three candidates: %s", visited, measured.plan)
	}
	t.Logf("statistics=%s unrelated_messages=10000 shared_hits=%d shared_reads=%d execution_ms=%.3f plan=%s", statistics, plans[0].Plan.SharedHits, plans[0].Plan.SharedReads, plans[0].ExecutionMS, measured.plan)
	return nil
}
