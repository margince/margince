// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
)

// planNode is the part of an EXPLAIN (FORMAT JSON) node this test reads.
type planNode struct {
	NodeType  string     `json:"Node Type"`  //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	Alias     string     `json:"Alias"`      //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	IndexName string     `json:"Index Name"` //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	Plans     []planNode `json:"Plans"`      //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
}

// reachedBy reports whether any node of the subtree is a scan of the index and
// whether any is a sequential scan.
func (n planNode) reachedBy(index string) (usesIndex, scansSeq bool) {
	usesIndex = n.IndexName == index
	scansSeq = n.NodeType == "Seq Scan"
	for _, child := range n.Plans {
		childIndex, childSeq := child.reachedBy(index)
		usesIndex = usesIndex || childIndex
		scansSeq = scansSeq || childSeq
	}
	return usesIndex, scansSeq
}

// citedLookups collects the nodes that read the scout's citation alias.
func (n planNode) citedLookups() []planNode {
	var out []planNode
	if n.Alias == "ca" {
		out = append(out, n)
	}
	for _, child := range n.Plans {
		out = append(out, child.citedLookups()...)
	}
	return out
}

// The scout's signal branch checks, for every citation of every open signal,
// that the cited activity is still live. That lookup must reach activity by its
// primary key: comparing text scans the whole table once per citation, so its
// cost grows with the table instead of with the citations.
//
// Sequential scans are switched off so the plan cannot pick one for being cheap
// on a test-sized table: with them off, a lookup that CAN use the key does, and
// one that cannot is left with the scan this test then refuses.
func TestTheScoutReachesACitedActivityByItsPrimaryKey(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC()
	// The planner prices a lookup from the table's statistics, and an empty
	// table has none: it would pick whatever is cheapest on zero rows.
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO activity (kind, subject, source, captured_by, occurred_at)
		SELECT 'email', 'plan seed ' || g, 'manual', 'human:test', now() - (g || ' minutes')::interval
		  FROM generate_series(1, 5000) g`); err != nil {
		t.Fatalf("seeding activities: %v", err)
	}
	if _, err := owner.Exec(context.Background(), `ANALYZE activity`); err != nil {
		t.Fatalf("analyzing activities: %v", err)
	}
	query, args := dealScoutSQL(now.Add(-dealScoutWindow), now, dealScoutCompanyCap)

	var raw []byte
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `EXPLAIN (FORMAT JSON) `+query, args...).Scan(&raw)
	}); err != nil {
		t.Fatalf("planning the scout's read: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"` //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}

	lookups := plans[0].Plan.citedLookups()
	if len(lookups) == 0 {
		t.Fatalf("the plan has no lookup of the cited activity — the statement changed shape:\n%s", raw)
	}
	for _, lookup := range lookups {
		usesKey, scansSeq := lookup.reachedBy("activity_pkey")
		if !usesKey || scansSeq {
			t.Errorf("a cited activity is not reached by its primary key (uses key: %v, scans the table: %v):\n%s", usesKey, scansSeq, raw)
		}
	}
}
