// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

type populationQueryCounts struct {
	mu      sync.Mutex
	queries map[string]int
}

func (c *populationQueryCounts) observe(sql string) {
	query := strings.Join(strings.Fields(sql), " ")
	kind := ""
	switch {
	case strings.HasPrefix(query, "SELECT display_name FROM app_user WHERE id = $1 AND "):
		kind = "owner"
	case query == "SELECT name FROM team WHERE id = $1 AND archived_at IS NULL":
		kind = "team"
	case strings.HasPrefix(query, "SELECT EXISTS ( SELECT 1 FROM team_membership tm "):
		kind = "membership"
	case query == "SELECT current_setting('transaction_isolation')":
		kind = "isolation"
	}
	if kind != "" {
		c.mu.Lock()
		c.queries[kind]++
		c.mu.Unlock()
	}
}

func (c *populationQueryCounts) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.observe(data.SQL)
	return ctx
}

func (*populationQueryCounts) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ *pgx.Batch) context.Context {
	return ctx
}
func (c *populationQueryCounts) TraceBatchQuery(_ context.Context, _ *pgx.Conn, data pgx.TraceBatchQueryData) {
	c.observe(data.SQL)
}
func (*populationQueryCounts) TraceBatchEnd(context.Context, *pgx.Conn, error) {}

func (*populationQueryCounts) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *populationQueryCounts) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queries = make(map[string]int)
}

func (c *populationQueryCounts) count(kind string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.queries[kind]
}

func traceReportingPopulation(t *testing.T, e *forecastEnv) *populationQueryCounts {
	t.Helper()
	cfg, err := database.PoolConfig(os.Getenv("MARGINCE_TEST_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns, cfg.MinConns = 2, 0
	counts := &populationQueryCounts{queries: make(map[string]int)}
	cfg.ConnConfig.Tracer = multitracer.New(cfg.ConnConfig.Tracer, counts)
	pool, err := testdb.OwnPoolFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	e.Pool = pool
	return counts
}

func TestReportingPopulationLabelsAreReadOnceAcrossRealMetrics(t *testing.T) {
	e := setupForecast(t)
	closed, _, _ := reportingWrittenClose(t, e)
	if closed.ClosedAt == nil {
		t.Fatal("production closing has no timestamp")
	}
	counts := traceReportingPopulation(t, e)
	at := closed.ClosedAt.Add(2 * time.Hour)
	service := newReportingService(e.Pool, func() time.Time { return at })
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	selection := crmcontracts.ReportingSelection{
		Scope:  crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)},
		Period: "this_month", TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "closed_win_rate", "open_pipeline"},
		Blocks:  []crmcontracts.ReportingBlockKind{"bookings_trend", "stage_distribution"},
	}
	var previous *crmcontracts.ReportingEvaluation
	for range 2 {
		counts.reset()
		result, err := service.Evaluate(ctx, selection)
		if err != nil {
			t.Fatal(err)
		}
		if counts.count("team") != 1 || counts.count("isolation") != 1 {
			t.Fatalf("three metrics read team=%d isolation=%d; want one each", counts.count("team"), counts.count("isolation"))
		}
		if len(result.Metrics) != 3 || result.Metrics[0].Value == nil || *result.Metrics[0].Value != 21600000 {
			t.Fatalf("production closing was not counted: %+v", result.Metrics)
		}
		if previous != nil && !reflect.DeepEqual(*previous, result) {
			t.Fatal("a new snapshot changed the fixed evaluation or its receipt")
		}
		previous = &result
	}
	if previous == nil {
		t.Fatal("no evaluated reading")
	}
	evidence, err := service.Evidence(ctx, selection, "bookings_won", "interval", "", nil, nil, 50,
		reporting.EvidenceExpectation{Key: previous.EvaluationKey, At: at, FrameworkRevision: previous.Context.FrameworkRevision})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) != 1 || evidence.Rows[0].SourceId == nil || *evidence.Rows[0].SourceId != closed.Id {
		t.Fatalf("evidence did not identify the production closing: %+v", evidence.Rows)
	}
	selection.Scope = crmcontracts.ReportingScope{Kind: "workspace"}
	counts.reset()
	if _, err := service.Evaluate(ctx, selection); err != nil {
		t.Fatal(err)
	}
	if counts.count("isolation") != 0 || counts.count("team") != 0 || counts.count("owner") != 0 {
		t.Fatalf("workspace evaluation performed population cache reads: %+v", counts.queries)
	}
}

func TestReportingPopulationReadsRespectReadCommitted(t *testing.T) {
	e := setupForecast(t)
	counts := traceReportingPopulation(t, e)
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	if err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.ReadCommitted, func(tx pgx.Tx) error {
		cached, release := withReportingPopulationReads(ctx, tx)
		defer release()
		counts.reset()
		for range 2 {
			if _, err := ResolveAnalyticsScope(cached, tx, RequestedScope{Kind: ScopeKindTeam, ID: &e.Team1}); err != nil {
				return err
			}
		}
		if counts.count("team") != 2 || counts.count("isolation") != 1 {
			t.Fatalf("read-committed team=%d isolation=%d; want 2 and 1", counts.count("team"), counts.count("isolation"))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReportingPopulationReadsEndWithTheirEvaluation(t *testing.T) {
	e := setupForecast(t)
	counts := traceReportingPopulation(t, e)
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	if err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		cached, release := withReportingPopulationReads(ctx, tx)
		defer release()
		counts.reset()
		if _, err := analyticsTeamLabel(cached, tx, e.Team1); err != nil {
			return err
		}
		if err := InstallationDB(e.Pool).TxIsolated(cached, pgx.RepeatableRead, func(other pgx.Tx) error {
			if other.Conn() == tx.Conn() {
				t.Fatal("separate transaction reused the held connection")
			}
			_, err := analyticsTeamLabel(cached, other, e.Team1)
			return err
		}); err != nil {
			return err
		}
		if counts.count("team") != 2 {
			t.Fatal("another transaction reused the evaluation's subject")
		}
		release()
		if _, err := analyticsTeamLabel(cached, tx, e.Team1); err != nil {
			return err
		}
		if counts.count("team") != 3 {
			t.Fatalf("released context reused a subject: %d reads", counts.count("team"))
		}
		fresh, closeFresh := withReportingPopulationReads(cached, tx)
		defer closeFresh()
		if _, err := analyticsTeamLabel(fresh, tx, e.Team1); err != nil {
			return err
		}
		if counts.count("team") != 4 {
			t.Fatal("a separate evaluation reused another evaluation's subject")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// The same evaluation in a read-committed transaction reads every subject
// again each time it is resolved, so it is the uncached baseline. The
// statement meter shows what a repeatable-read evaluation no longer sends,
// and both evaluations must answer the same.
func TestReportingPopulationStatementMeterShowsTheSavedReads(t *testing.T) {
	e := setupForecast(t)
	closed, _, _ := reportingWrittenClose(t, e)
	counts := traceReportingPopulation(t, e)
	at := closed.ClosedAt.Add(2 * time.Hour)
	service := newReportingService(e.Pool, func() time.Time { return at })
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	selection := crmcontracts.ReportingSelection{
		Scope:  crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)},
		Period: "this_month", TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "closed_win_rate", "open_pipeline"},
		Blocks:  []crmcontracts.ReportingBlockKind{"bookings_trend", "stage_distribution"},
	}
	type reading struct {
		result     crmcontracts.ReportingEvaluation
		statements int64
		teamReads  int
	}
	evaluate := func(level pgx.TxIsoLevel) reading {
		t.Helper()
		counts.reset()
		metered, meter := database.WithStatementMeter(ctx)
		var out reading
		if err := InstallationDB(e.Pool).TxIsolated(metered, level, func(tx pgx.Tx) error {
			before := meter.Cost()
			var err error
			out.result, err = service.EvaluateTx(metered, tx, selection)
			out.statements = meter.Cost().Minus(before).Statements
			return err
		}); err != nil {
			t.Fatal(err)
		}
		out.teamReads = counts.count("team")
		return out
	}
	uncached := evaluate(pgx.ReadCommitted)
	cached := evaluate(pgx.RepeatableRead)
	if uncached.teamReads < 3 {
		t.Fatalf("the read-committed baseline read the team %d times; the test needs a repeated read to measure", uncached.teamReads)
	}
	if cached.teamReads != 1 {
		t.Fatalf("the repeatable-read evaluation read the team %d times; want 1", cached.teamReads)
	}
	if saved := uncached.statements - cached.statements; saved != int64(uncached.teamReads-1) {
		t.Fatalf("statements: read committed %d, repeatable read %d; want %d fewer", uncached.statements, cached.statements, uncached.teamReads-1)
	}
	if !reflect.DeepEqual(uncached.result, cached.result) {
		t.Fatalf("cached and uncached evaluations differ:\nuncached %+v\ncached   %+v", uncached.result, cached.result)
	}
	t.Logf("team-scope evaluation: %d statements uncached, %d cached; team label read %d times uncached, once cached",
		uncached.statements, cached.statements, uncached.teamReads)
}
