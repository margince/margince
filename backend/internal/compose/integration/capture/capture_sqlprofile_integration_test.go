// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
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
	"github.com/margince/margince/backend/internal/platform/testdb"
)

type historyQueryCost struct {
	query   string
	calls   int
	elapsed time.Duration
	args    []any
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

func (p *historySQLProfile) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	span, ok := ctx.Value(historyQuerySpanKey{}).(historyQuerySpan)
	if !ok {
		return
	}
	elapsed := time.Since(span.began)
	p.mu.Lock()
	defer p.mu.Unlock()
	cost := p.queries[span.query]
	if cost.calls == 0 {
		cost.args = span.args
	}
	cost.query, cost.calls, cost.elapsed = span.query, cost.calls+1, cost.elapsed+elapsed
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
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			for i, cost := range costs {
				query := strings.TrimSpace(cost.query)
				if i >= 2 && !strings.HasPrefix(query, "WITH perfect AS") {
					continue
				}
				if !strings.HasPrefix(query, "WITH candidate") && !strings.HasPrefix(query, "WITH perfect AS") && !strings.HasPrefix(query, "WITH emails AS") && !strings.HasPrefix(query, "SELECT EXISTS") {
					continue
				}
				var plan []byte
				err := database.WithWorkspaceTx(ctx, measured, func(tx pgx.Tx) error {
					return tx.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+cost.query, cost.args...).Scan(&plan)
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("query-%d.json", i)), plan, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
		for _, cost := range costs[:min(20, len(costs))] {
			logHistoryQueryCost(t, cost)
		}
	})
	return measured
}
