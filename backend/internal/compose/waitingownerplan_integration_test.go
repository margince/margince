// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The who-is-waiting statement looks up who owes each reply one link at a time.
//
// As four plain joins the planner was free to treat the owner tables as a flat
// relation, and on production's data it did: underestimating the candidates
// ninefold, it rescanned all of contact and company once per candidate and
// spent 3.6 of the query's 4.1 seconds discarding 27 million rows. The lookups
// are fenced subqueries keyed on the link column, and this holds that shape on
// the statement the store actually sends.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
)

// waitingStatement keeps the who-is-waiting statement and its arguments as the
// store sent them, so the plan is of production's SQL rather than a copy.
type waitingStatement struct {
	mu   sync.Mutex
	sql  string
	args []any
}

func (w *waitingStatement) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "LEFT JOIN activity_participant sender") {
		w.mu.Lock()
		w.sql, w.args = data.SQL, data.Args
		w.mu.Unlock()
	}
	return ctx
}

func (*waitingStatement) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

type waitingPlanNode struct {
	Alias     string            `json:"Alias"`
	IndexCond string            `json:"Index Cond"` //nolint:tagliatelle // fixed by the server's plan format
	Filter    string            `json:"Filter"`
	Plans     []waitingPlanNode `json:"Plans"`
}

func (n waitingPlanNode) walk(visit func(waitingPlanNode)) {
	visit(n)
	for _, child := range n.Plans {
		child.walk(visit)
	}
}

func TestTheWaitingQueryLooksUpEachReplyOwnerByItsLink(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "dana@acme.example")
	seedWaitingOfKind(t, e, contact, "email", "Re: the renewal quote", time.Now().Add(-72*time.Hour))

	sent := &waitingStatement{}
	config := e.Pool.Config()
	config.ConnConfig.Tracer = sent
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("opening a traced pool: %v", err)
	}
	defer pool.Close()
	seam := attentionWaiting{store: activities.NewStore(InstallationDB(pool)), now: time.Now}
	if _, _, err := seam.Unanswered(e.Admin(), time.Now()); err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	if sent.sql == "" {
		t.Fatal("the who-is-waiting statement was never sent, so there is no plan to hold")
	}

	raw := planUnderProductionCosts(t, e.Pool, sent)
	var plans []struct {
		Plan waitingPlanNode `json:"Plan"` //nolint:tagliatelle // fixed by the server's plan format
	}
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}

	owners := map[string]string{
		"ownerdeal": "wl.deal_id", "ownerlead": "wl.lead_id",
		"ownercontact": "wl.contact_id", "ownercompany": "wl.company_id",
	}
	probed := map[string]bool{}
	plans[0].Plan.walk(func(n waitingPlanNode) {
		if column, ok := owners[n.Alias]; ok && (strings.Contains(n.IndexCond, column) || strings.Contains(n.Filter, column)) {
			probed[n.Alias] = true
		}
	})
	for alias, column := range owners {
		if !probed[alias] {
			t.Errorf("%s is not read by %s: as a join it can rescan the whole table once per waiting message", alias, column)
		}
	}
}

// planUnderProductionCosts plans the statement as production's planner saw it.
// A seed of one message makes every lookup cheap, so the planner is told what
// production's misestimate told it: index probes are dear and hash or merge
// joins unavailable. A join over the owner tables then rescans them, and only
// a lookup keyed on its link still reads by it.
func planUnderProductionCosts(t *testing.T, pool *pgxpool.Pool, sent *waitingStatement) string {
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
		SET LOCAL enable_bitmapscan = off; SET LOCAL random_page_cost = 1000`); err != nil {
		t.Fatalf("setting the planner's costs: %v", err)
	}
	var raw string
	if err := tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+sent.sql, sent.args...).Scan(&raw); err != nil {
		t.Fatalf("planning the who-is-waiting statement: %v", err)
	}
	return raw
}
