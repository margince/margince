// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// The 360 exists to replace a stack of round trips with one. That promise
// is only true if the assembly's cost is flat in the size of the account:
// a per-contact strength read, or a per-deal stage lookup, would turn one
// HTTP call into N database calls and quietly reintroduce exactly what the
// endpoint was built to remove.
//
// So this counts queries rather than milliseconds. A timing assertion on a
// shared CI box measures the box; a query count measures the code, and it
// is the number that actually degrades when someone adds a loop.

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	company360svc "github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// countingTracer counts the statements one pool issues, ignoring the
// transaction and GUC plumbing every workspace transaction pays once —
// what is under test is how the section reads scale, not BEGIN.
type countingTracer struct {
	mu            sync.Mutex
	count         int
	languageReads int
}

func (t *countingTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	sql := strings.TrimSpace(strings.ToUpper(data.SQL))
	if strings.HasPrefix(sql, "BEGIN") || strings.HasPrefix(sql, "COMMIT") ||
		strings.HasPrefix(sql, "ROLLBACK") || strings.HasPrefix(sql, "SELECT SET_CONFIG") {
		return ctx
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.count++
	for _, arg := range data.Args {
		if key, ok := arg.(string); ok && key == identity.BaseLanguage.Key() {
			t.languageReads++
		}
	}
	return ctx
}

func (t *countingTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (t *countingTracer) total() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

// tracedPool opens a second app-role pool that counts its statements.
func tracedPool(t *testing.T, tracer *countingTracer) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_APP_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Tracer = tracer
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		database.RegisterIDTypes(conn)
		return nil
	}
	traced, err := testdb.OwnPoolFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(traced.Close)
	return traced
}

func TestCompany360CostDoesNotGrowWithTheAccount(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)

	// One pipeline for both accounts: the fixture seeds the workspace
	// default, and seeding it twice is a conflict, not a second pipeline.
	pipeline, stage, _ := integration.DealFixture(t, e)
	small := seedAccount(t, e, owner, "Small Account", 1, pipeline, stage)
	large := seedAccount(t, e, owner, "Large Account", 12, pipeline, stage)

	tracer := &countingTracer{}
	pool := tracedPool(t, tracer)
	traced := database.BindTo(pool, ids.From[ids.WorkspaceKind](e.WS))
	svc := company360svc.NewService(pool, contacts.NewStore(traced),
		deals.NewStore(traced, installseam.Deals()), integration.ProjectsStore(traced),
		approvals.NewService(traced), func() time.Time { return company360Clock })
	ctx := e.Admin()

	before := tracer.total()
	if _, err := svc.Assemble(ctx, small); err != nil {
		t.Fatalf("assemble the small account: %v", err)
	}
	smallCost := tracer.total() - before

	before = tracer.total()
	if _, err := svc.Assemble(ctx, large); err != nil {
		t.Fatalf("assemble the large account: %v", err)
	}
	largeCost := tracer.total() - before

	if largeCost != smallCost {
		t.Errorf("assembling an account with 12 contacts and 12 deals cost %d queries, one with 1 cost %d — "+
			"the composite read must be flat in the size of the account, or it is the round-trip stack it replaced",
			largeCost, smallCost)
	}
	// Every timeline page reads commitment admission evidence in one batch.
	// The equality above also holds this cost flat as the account grows.
	// Shared prose adds one language read, reused by every section, and the
	// day counts add one installation zone read. The commitments section adds
	// the read of what they promised and its completeness count; the read of
	// ours is shared with the moment card.
	const budget = 55
	if tracer.languageReads != 2 {
		t.Errorf("two assemblies read base language %d times, want once each", tracer.languageReads)
	}
	if smallCost > budget {
		t.Errorf("one 360 issued %d queries, budget is %d", smallCost, budget)
	}
}

// seedAccount builds one company with n employed contacts (each with
// an interaction, so strength is real work) and n open deals.
func seedAccount(t *testing.T, e *integration.Env, owner *pgx.Conn, name string, n int,
	pipeline ids.PipelineID, stage ids.StageID,
) ids.CompanyID {
	t.Helper()
	company := e.SeedCompany(t, name, &e.Rep1)
	for range n {
		contact := e.SeedContact(t, name+" contact", &e.Rep1)
		e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
			VALUES ('employment', $1, $2, 'manual', 'human:x')`, contact, company)
		activity := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
			VALUES ($1, 'email', 'touch', '2026-05-28T09:00:00Z', 'inbound', 'manual', 'human:x')`)
		integration.LinkActivity(t, owner, activity, "contact", contact)

		deal := e.SeedDeal(t, name+" deal", pipeline, stage, &e.Rep1)
		e.WsExec(t, `UPDATE deal SET company_id = $2 WHERE id = $1`, deal, company)
		e.WsExec(t, `INSERT INTO relationship (kind, contact_id, deal_id, role, source, captured_by)
			VALUES ('deal_stakeholder', $1, $2, 'champion', 'manual', 'human:x')`, contact, deal)
	}
	return ids.From[ids.CompanyKind](company)
}
