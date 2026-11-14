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
	best := time.Duration(-1)
	for range 3 {
		rows, err := e.Pool.Query(context.Background(), "EXPLAIN (GENERIC_PLAN, COSTS false, SUMMARY) "+sql,
			pgx.QueryExecModeSimpleProtocol)
		if err != nil {
			return 0, false
		}
		for rows.Next() {
			var line string
			if rows.Scan(&line) != nil {
				continue
			}
			if m := planningLine.FindStringSubmatch(line); m != nil {
				planned, _ := strconv.ParseFloat(m[1], 64)
				if took := time.Duration(planned * float64(time.Millisecond)); best < 0 || took < best {
					best = took
				}
			}
		}
		rows.Close()
		if rows.Err() != nil {
			return 0, false
		}
	}
	return best, best >= 0
}
