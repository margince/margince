// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// homePlanning is how much of one read went to planning. Postgres plans a
// statement again on each run while it keeps a custom plan; a chosen generic
// plan is planned once per connection.
type homePlanning struct {
	statements, replanned, unexplained int
	replanning, firstPlans             time.Duration
}

// logHomePlanning runs each timed read once more, untimed, after its kept answer
// is forgotten. It names the statements the read ran and those Postgres planned
// again, and how long their planning takes. A cold read plans every statement
// once, so firstPlans is what planning adds to it.
func logHomePlanning(t *testing.T, e *apptest.AppEnv, s Seat, timed []dailyCall, results []DailyResult) {
	t.Helper()
	p50 := map[string]time.Duration{}
	for _, r := range results {
		for _, call := range timed {
			if r.Row == dailyRowName(homeMissFlow.Name, call) {
				p50[call.Name] = r.P50
			}
		}
	}
	for _, call := range timed {
		forgetHomeAnswers(t, e, s)
		before := preparedPlans(t, e)
		if status, _, _ := s.Get(t, e, call.Path); status != http.StatusOK {
			t.Fatalf("planning %s answered %d", call.Name, status)
		}
		got := homePlanningOf(t, e, before, preparedPlans(t, e))
		t.Logf("home-budget: plan %-10s statements=%d replanned=%d replanning=%.1fms first-plans=%.1fms unexplained=%d miss-p50=%dms",
			call.Name, got.statements, got.replanned, ms(got.replanning), ms(got.firstPlans), got.unexplained,
			p50[call.Name].Milliseconds())
	}
}

func homePlanningOf(t *testing.T, e *apptest.AppEnv, before, after map[preparedStatement]planCount) homePlanning {
	t.Helper()
	texts := preparedTexts(t, e)
	var out homePlanning
	timed := map[string]time.Duration{}
	for statement, now := range after {
		then := before[statement]
		if now.custom+now.generic == then.custom+then.generic {
			continue
		}
		out.statements++
		took, seen := timed[statement.name]
		if !seen {
			var ok bool
			if took, ok = planningTime(e, texts[statement]); !ok {
				out.unexplained++
			}
			timed[statement.name] = took
		}
		out.firstPlans += took
		if now.custom > then.custom {
			out.replanned++
			out.replanning += took
		}
	}
	return out
}

// preparedTexts reads the SQL of every statement prepared on the pool's
// connections. The simple protocol prepares nothing, so it reads only the app's.
func preparedTexts(t *testing.T, e *apptest.AppEnv) map[preparedStatement]string {
	t.Helper()
	ctx := context.Background()
	idle := e.Pool.AcquireAllIdle(ctx)
	defer func() {
		for _, conn := range idle {
			conn.Release()
		}
	}()
	texts := map[preparedStatement]string{}
	for _, conn := range idle {
		rows, err := conn.Query(ctx, `SELECT name, statement FROM pg_prepared_statements`, pgx.QueryExecModeSimpleProtocol)
		if err != nil {
			t.Fatalf("reading prepared statements: %v", err)
		}
		pid := conn.Conn().PgConn().PID()
		for rows.Next() {
			var name, text string
			if err := rows.Scan(&name, &text); err != nil {
				t.Fatalf("reading prepared statements: %v", err)
			}
			texts[preparedStatement{pid, name}] = text
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("reading prepared statements: %v", err)
		}
	}
	return texts
}

var planningLine = regexp.MustCompile(`^Planning Time: ([0-9.]+) ms$`)

// planningTime plans sql without its arguments, as the app's role, and reports
// the fastest of three runs. A plan without arguments is a generic plan, so the
// time is an estimate of a custom plan's. A statement Postgres cannot plan
// without arguments reports false.
func planningTime(e *apptest.AppEnv, sql string) (time.Duration, bool) {
	if sql == "" {
		return 0, false
	}
	ctx := context.Background()
	conn, err := e.Pool.Acquire(ctx)
	if err != nil {
		return 0, false
	}
	defer conn.Release()
	best := time.Duration(-1)
	for range 3 {
		// The raw simple protocol sends the text as it is, so its $n stay parameters.
		results, err := conn.Conn().PgConn().Exec(ctx, "EXPLAIN (GENERIC_PLAN, COSTS false, SUMMARY) "+sql).ReadAll()
		if err != nil || len(results) != 1 || results[0].Err != nil {
			return 0, false
		}
		for _, row := range results[0].Rows {
			m := planningLine.FindSubmatch(row[0])
			if m == nil {
				continue
			}
			planned, err := strconv.ParseFloat(string(m[1]), 64)
			if err != nil {
				return 0, false
			}
			if took := time.Duration(planned * float64(time.Millisecond)); best < 0 || took < best {
				best = took
			}
		}
	}
	return best, best >= 0
}
