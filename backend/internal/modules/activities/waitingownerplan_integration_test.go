// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The who-is-waiting statement keys each reply-owner lookup on its link.
//
// As four plain joins the planner was free to treat the owner tables as a flat
// relation, and on production's data it did: underestimating the candidates
// ninefold, it rescanned all of contact and company once per candidate and
// spent 3.6 of the query's 4.1 seconds discarding 27 million rows in a join
// filter. The lookups are fenced laterals, and this holds that shape on the
// statement the store actually sends.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// waitingStatement keeps the who-is-waiting statement and its arguments as the
// store sent them, so the plan is of production's SQL rather than a copy. The
// store reads on the test's own goroutine, so it needs no lock.
type waitingStatement struct {
	sql  string
	args []any
}

func (w *waitingStatement) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "LEFT JOIN activity_participant sender") {
		w.sql, w.args = data.SQL, data.Args
	}
	return ctx
}

func (*waitingStatement) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestTheWaitingQueryKeysEachReplyOwnerLookupOnItsLink(t *testing.T) {
	e := setupLoad(t)
	sent := &waitingStatement{}
	config := e.pool.Config()
	config.ConnConfig.Tracer = sent
	pool, err := testdb.OwnPoolFromConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("opening a traced pool: %v", err)
	}
	defer pool.Close()
	store := NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))
	if _, err := store.WaitingReplies(e.as(), time.Now()); err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	if sent.sql == "" {
		t.Fatal("the who-is-waiting statement was never sent, so there is no plan to hold")
	}

	var plans []struct {
		Plan planNode `json:"Plan"` //nolint:tagliatelle // fixed by the server's plan format
	}
	if err := json.Unmarshal([]byte(planWithoutKeyedJoins(t, e.pool, sent)), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}
	owners := map[string]string{
		"ownerdeal": "wl.deal_id", "ownerlead": "wl.lead_id",
		"ownercontact": "wl.contact_id", "ownercompany": "wl.company_id",
	}
	keyed := map[string]bool{}
	plans[0].Plan.walk(func(n planNode) {
		if column, ok := owners[n.Alias]; ok && (strings.Contains(n.IndexCond, column) || strings.Contains(n.Filter, column)) {
			keyed[n.Alias] = true
		}
	})
	for alias, column := range owners {
		if !keyed[alias] {
			t.Errorf("%s is not looked up by %s: a join over it is judged after reading the table, not keyed on the link", alias, column)
		}
	}
}

// planWithoutKeyedJoins plans the statement with every way to key a join taken
// away: no index or bitmap probe, no hash or merge join. A plain join over an owner
// table can then only leave the link comparison in a join filter, which is the shape
// production's misestimate chose; a lateral keyed on its link carries the comparison
// into its own scan instead.
//
// ANALYZED first, because choosing between those two is still a COST decision — this
// test used to claim it depended on neither rows nor statistics, and in the parallel
// lane the estimates are whatever the instance last computed while seven other
// packages wrote to the same tables. Inside the planning transaction the deferred
// Rollback undoes, so nothing leaks into the next test.
func planWithoutKeyedJoins(t *testing.T, pool *pgxpool.Pool, sent *waitingStatement) string {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the planning transaction: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("closing the planning transaction: %v", err)
		}
	}()
	if _, err := tx.Exec(ctx, `SET LOCAL enable_hashjoin = off; SET LOCAL enable_mergejoin = off;
		SET LOCAL enable_indexscan = off; SET LOCAL enable_indexonlyscan = off;
		SET LOCAL enable_bitmapscan = off`); err != nil {
		t.Fatalf("setting the planner's costs: %v", err)
	}
	// The whole database rather than a list. The statement reaches further than its
	// joins — owedSQL and requestCandidateSQL bring in activity_participant,
	// activity_reader_state, contact_email and two more — and every one of their
	// estimates feeds the outer row count that decides this plan. A hand-kept list
	// is a completeness claim that was already wrong once.
	if _, err := tx.Exec(ctx, `ANALYZE`); err != nil {
		t.Fatalf("analyzing before planning: %v", err)
	}
	var raw string
	if err := tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+sent.sql, sent.args...).Scan(&raw); err != nil {
		t.Fatalf("planning the who-is-waiting statement: %v", err)
	}
	return raw
}
