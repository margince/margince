// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

type historyQueryCost struct {
	query   string
	calls   int
	elapsed time.Duration
	args    []any
	pid     uint32
}

type historySQLProfile struct {
	mu      sync.Mutex
	queries map[string]historyQueryCost
	report  func(historyQueryCost)
}

type historyQuerySpan struct {
	query string
	began time.Time
	args  []any
}

type historyQuerySpanKey struct{}

func (p *historySQLProfile) TraceQueryStart(ctx context.Context, _ *pgx.Conn, q pgx.TraceQueryStartData) context.Context {
	if _, metered := database.StatementMeterOf(ctx); !metered {
		return ctx
	}
	return context.WithValue(ctx, historyQuerySpanKey{}, historyQuerySpan{query: q.SQL, began: time.Now(), args: append([]any(nil), q.Args...)})
}

func (p *historySQLProfile) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, _ pgx.TraceQueryEndData) {
	span, ok := ctx.Value(historyQuerySpanKey{}).(historyQuerySpan)
	if !ok {
		return
	}
	elapsed := time.Since(span.began)
	p.mu.Lock()
	defer p.mu.Unlock()
	cost := p.queries[span.query]
	cost.args = span.args
	cost.query, cost.calls, cost.elapsed = span.query, cost.calls+1, cost.elapsed+elapsed
	cost.pid = conn.PgConn().PID()
	p.queries[span.query] = cost
	if p.report != nil {
		p.report(cost)
	}
}

func logHistoryQueryCost(t *testing.T, cost historyQueryCost) {
	t.Helper()
	query := strings.Join(strings.Fields(cost.query), " ")
	if len(query) > 180 {
		query = query[:180]
	}
	t.Logf("sql_profile calls=%d elapsed=%s query=%q", cost.calls, cost.elapsed, query)
}

func profileHistoryPool(ctx context.Context, t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	profile := &historySQLProfile{queries: map[string]historyQueryCost{}}
	// A package timeout bypasses cleanup, so long runs also report their completed work.
	profile.report = func(cost historyQueryCost) {
		if cost.calls%1000 == 0 || cost.calls&(cost.calls-1) == 0 {
			logHistoryQueryCost(t, cost)
		}
	}
	config := pool.Config()
	if os.Getenv("MARGINCE_IMPORT_BENCH_PREPARED") != "" {
		config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
		config.ConnConfig.StatementCacheCapacity = 256
	}
	if os.Getenv("MARGINCE_IMPORT_BENCH_PLAN_DIR") != "" {
		config.MaxConnLifetime, config.MaxConnLifetimeJitter, config.MaxConnIdleTime = 0, 0, 0
	}
	config.ConnConfig.Tracer = multitracer.New(config.ConnConfig.Tracer, profile)
	measured, err := testdb.OwnPoolFromConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer measured.Close()
		var costs []historyQueryCost
		for _, cost := range profile.queries {
			costs = append(costs, cost)
		}
		sort.Slice(costs, func(i, j int) bool { return costs[i].elapsed > costs[j].elapsed })
		if len(costs) == 0 {
			t.Error("no metered SQL was profiled")
			return
		}
		if directory := os.Getenv("MARGINCE_IMPORT_BENCH_PLAN_DIR"); directory != "" {
			writeHistoryQueryPlans(ctx, t, measured, directory, costs)
		}
		for _, cost := range costs[:min(20, len(costs))] {
			logHistoryQueryCost(t, cost)
		}
	})
	return measured
}

func writeHistoryQueryPlans(ctx context.Context, t *testing.T, measured *pgxpool.Pool, directory string, costs []historyQueryCost) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	namePlanSeen := false
	for i, cost := range costs {
		query := strings.TrimSpace(cost.query)
		clockRead := strings.HasPrefix(query, "SELECT max(a.occurred_at)")
		if i >= 5 && !strings.HasPrefix(query, "WITH perfect AS") && !clockRead {
			continue
		}
		if !strings.HasPrefix(query, "WITH candidate") && !strings.HasPrefix(query, "WITH perfect AS") && !strings.HasPrefix(query, "WITH emails AS") && !strings.HasPrefix(query, "SELECT EXISTS") && !clockRead {
			continue
		}
		var plan, genericPlan, customPlan []byte
		readPlan := func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SET TRANSACTION READ ONLY`); err != nil {
				return err
			}
			if os.Getenv("MARGINCE_IMPORT_BENCH_PREPARED") == "" {
				return tx.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+cost.query, cost.args...).Scan(&plan)
			}
			statementQuery, values := cost.query, cost.args
			if len(values) == 1 {
				if named, ok := values[0].(pgx.NamedArgs); ok {
					var err error
					statementQuery, values, err = named.RewriteQuery(ctx, tx.Conn(), statementQuery, nil)
					if err != nil {
						return err
					}
				}
			}
			var statement string
			var parameterTypes []uint32
			if err := tx.QueryRow(ctx, `SELECT name, parameter_types::oid[] FROM pg_prepared_statements WHERE statement=$1`, statementQuery).Scan(&statement, &parameterTypes); err != nil {
				return fmt.Errorf("locating the measured cached statement: %w", err)
			}
			if len(values) != len(parameterTypes) {
				return fmt.Errorf("cached statement has %d parameters for %d values", len(parameterTypes), len(values))
			}
			encoded := make([]any, len(values))
			for i, value := range values {
				text, err := tx.Conn().TypeMap().Encode(parameterTypes[i], pgx.TextFormatCode, value, nil)
				if err != nil {
					return fmt.Errorf("encoding cached parameter %d: %w", i, err)
				}
				if text != nil {
					encoded[i] = string(text)
				}
			}
			query := `EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) EXECUTE ` + pgx.Identifier{statement}.Sanitize() + `(` + storekit.Placeholders(values) + `)`
			arguments := append([]any{pgx.QueryExecModeSimpleProtocol}, encoded...)
			if err := tx.QueryRow(ctx, query, arguments...).Scan(&plan); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `SET LOCAL plan_cache_mode=force_generic_plan`); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, query, arguments...).Scan(&genericPlan); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `SET LOCAL plan_cache_mode=force_custom_plan`); err != nil {
				return err
			}
			return tx.QueryRow(ctx, query, arguments...).Scan(&customPlan)
		}
		err := func() error {
			if os.Getenv("MARGINCE_IMPORT_BENCH_PREPARED") == "" {
				return database.WithWorkspaceTx(ctx, measured, readPlan)
			}
			// Keep other backends acquired so the tenant transaction explains
			// the statement cached by the connection that actually ran it.
			idle := measured.AcquireAllIdle(ctx)
			defer func() {
				for _, conn := range idle {
					if conn != nil {
						conn.Release()
					}
				}
			}()
			for i, conn := range idle {
				if conn.Conn().PgConn().PID() == cost.pid {
					conn.Release()
					idle[i] = nil
					return database.WithWorkspaceTx(ctx, measured, readPlan)
				}
			}
			return errors.New("the measured statement's backend is unavailable")
		}()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("query-%d.json", i)), plan, 0o600); err != nil {
			t.Fatal(err)
		}
		if genericPlan != nil {
			if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("query-%d-generic.json", i)), genericPlan, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if customPlan != nil {
			if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("query-%d-custom.json", i)), customPlan, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if strings.HasPrefix(query, "WITH perfect AS") {
			namePlanSeen = true
			if err := checkHistoryNamePlans(cost, plan, genericPlan, customPlan); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !namePlanSeen {
		t.Fatal("capture profile did not report its name-candidate plan")
	}
}
