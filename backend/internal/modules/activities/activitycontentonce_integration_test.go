// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// projectionTrace keeps every statement that selects the activity projection.
type projectionTrace struct {
	mu         sync.Mutex
	statements []tracedStatement
}

type tracedStatement struct {
	sql  string
	args []any
}

func (p *projectionTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(d.SQL, "SELECT "+activityProjection[0].sql+", ") {
		p.mu.Lock()
		p.statements = append(p.statements, tracedStatement{d.SQL, append([]any(nil), d.Args...)})
		p.mu.Unlock()
	}
	return ctx
}

func (*projectionTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// last answers the newest projection statement and forgets the rest.
func (p *projectionTrace) last(t *testing.T) tracedStatement {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.statements) == 0 {
		t.Fatal("no activity projection statement ran")
	}
	got := p.statements[len(p.statements)-1]
	p.statements = nil
	return got
}

func (e *sendEnv) tracedStore(t *testing.T) (*Store, *projectionTrace) {
	t.Helper()
	trace := &projectionTrace{}
	cfg := e.pool.Config()
	cfg.ConnConfig.Tracer = trace
	cfg.MaxConns, cfg.MinConns = 2, 0
	pool, err := testdb.OwnPoolFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws))), trace
}

// The keys are Postgres's EXPLAIN JSON, not ours.
//
//nolint:tagliatelle // fixed by the server's plan format
type decisionPlanNode struct {
	NodeType     string             `json:"Node Type"`
	Relationship string             `json:"Parent Relationship"`
	ActualLoops  float64            `json:"Actual Loops"`
	Plans        []decisionPlanNode `json:"Plans"`
}

// decisionLoops runs the statement under EXPLAIN ANALYZE in the store's own
// transaction and answers how many times the content decision was evaluated.
func decisionLoops(ctx context.Context, t *testing.T, store *Store, statement tracedStatement) float64 {
	t.Helper()
	if n := strings.Count(statement.sql, "AS available OFFSET 0) content_decision"); n != 1 {
		t.Fatalf("statement carries %d content decisions, want 1:\n%s", n, statement.sql)
	}
	for _, payload := range []string{"a.body", chatSourceReadSQL(ctx, "a.raw", safeChatMetadataSQL)} {
		if !strings.Contains(statement.sql, "CASE WHEN (content_decision.available) THEN "+payload+" END") {
			t.Fatalf("%s is not gated by the shared decision:\n%s", payload, statement.sql)
		}
	}
	var raw string
	if err := store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+statement.sql, statement.args...).Scan(&raw)
	}); err != nil {
		t.Fatal(err)
	}
	var plans []struct {
		Plan decisionPlanNode `json:"Plan"` //nolint:tagliatelle // fixed by the server's plan format
	}
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}
	// The fenced LATERAL plans as an input-less Result on the join's inner side.
	// InitPlans and SubPlans are the gates' own work, so the walk skips them.
	var decisions []decisionPlanNode
	var walk func(decisionPlanNode)
	walk = func(n decisionPlanNode) {
		if n.NodeType == "Result" && n.Relationship == "Inner" && !hasJoinInput(n) {
			decisions = append(decisions, n)
		}
		for _, child := range n.Plans {
			if child.Relationship != "InitPlan" && child.Relationship != "SubPlan" {
				walk(child)
			}
		}
	}
	walk(plans[0].Plan)
	if len(decisions) != 1 {
		t.Fatalf("the plan has %d content decision nodes, want 1:\n%s", len(decisions), raw)
	}
	return decisions[0].ActualLoops
}

func hasJoinInput(n decisionPlanNode) bool {
	for _, child := range n.Plans {
		if child.Relationship == "Outer" || child.Relationship == "Inner" {
			return true
		}
	}
	return false
}

// Get, the per-deal batch and an ordinary page decide content once per row.
// A page decides it only for its own rows, not for every row that matched.
func TestTheContentDecisionRunsOnlyForTheSelectedRows(t *testing.T) {
	e := setupSend(t)
	ctx := principal.WithActor(e.as(principal.RowScopeAll), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Read: true}, "company": {Read: true}, "deal": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	store, trace := e.tracedStore(t)

	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	seed := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	seed(`INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	seed(`INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	seed(`INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Once deal', 'open', $2, $3, $4, 'manual', 'system')`, deal, e.rep, pipeline, stage)
	const matching = 4
	var first ids.UUID
	for i := range matching {
		body := "Body " + string(rune('A'+i))
		row, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "note", Subject: new("Once"), Body: &body, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = ids.UUID(row.Id)
		}
		seed(`INSERT INTO activity_link (id, activity_id, entity_type, deal_id) VALUES ($1, $2, 'deal', $3)`,
			ids.NewV7(), ids.UUID(row.Id), deal)
	}

	if _, err := store.GetActivity(ctx, ids.From[ids.ActivityKind](first), storekit.LiveOnly); err != nil {
		t.Fatal(err)
	}
	if loops := decisionLoops(ctx, t, store, trace.last(t)); loops != 1 {
		t.Fatalf("Get decided content %v times, want 1", loops)
	}

	in := ListActivitiesInput{Limit: new(1)}
	var lists map[ids.UUID]DealActivityList
	if err := store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		lists, err = ListActivitiesByDealTx(ctx, tx, []ids.UUID{deal}, in)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := lists[deal]; len(got.Activities) != 1 || !got.Truncated {
		t.Fatalf("deal list = %d rows, truncated %t; want 1 row, truncated", len(got.Activities), got.Truncated)
	}
	if loops := decisionLoops(ctx, t, store, trace.last(t)); loops < 1 || loops > 2 {
		t.Fatalf("the per-deal batch decided content %v times for a one-row page over %d matches", loops, matching)
	}

	rows, page, err := store.ListActivities(ctx, in)
	if err != nil || len(rows) != 1 || !page.HasMore {
		t.Fatalf("page = %d rows, has_more %t, %v", len(rows), page.HasMore, err)
	}
	if loops := decisionLoops(ctx, t, store, trace.last(t)); loops < 1 || loops > 2 {
		t.Fatalf("an ordinary page decided content %v times for a one-row page over %d matches", loops, matching)
	}
}
