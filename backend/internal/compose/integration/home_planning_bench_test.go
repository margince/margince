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

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// homePlanning is how much of one read went to planning, counted per run of a
// prepared statement on one connection. Postgres plans a custom plan on every
// run; a generic plan it keeps is planned once per connection.
type homePlanning struct {
	runs, customRuns, untimed  int64
	customPlanning, firstPlans time.Duration
}

// logHomePlanning runs each timed read once more, untimed, after its kept answer
// is forgotten. It counts the runs of the read's prepared statements, the runs
// on a custom plan, and how long their planning takes. A cold read plans every
// statement once, so first-plans is what planning adds to it. Untimed counts
// runs whose SQL Postgres could not plan without its arguments.
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
		got := homePlanningOf(e, before, preparedPlans(t, e))
		t.Logf("home-budget: plan %-10s runs=%d custom-plan-runs=%d custom-planning=%.1fms first-plans=%.1fms untimed-runs=%d miss-p50=%dms",
			call.Name, got.runs, got.customRuns, ms(got.customPlanning), ms(got.firstPlans), got.untimed,
			p50[call.Name].Milliseconds())
	}
}

// homePlanningOf weighs each run between the two readings by its statement's
// planning time. Both readings come from one query per connection, so the SQL
// and the counts belong to the same statement.
func homePlanningOf(e *apptest.AppEnv, before, after map[preparedStatement]planCount) homePlanning {
	var out homePlanning
	planned := map[string]time.Duration{}
	timed := map[string]bool{}
	for statement, now := range after {
		then := before[statement]
		runs := now.custom + now.generic - then.custom - then.generic
		if runs == 0 {
			continue
		}
		took, seen := planned[now.sql]
		if !seen {
			took, timed[now.sql] = planningTime(e, now.sql)
			planned[now.sql] = took
		}
		custom := now.custom - then.custom
		out.runs += runs
		out.customRuns += custom
		if !timed[now.sql] {
			out.untimed += runs
		}
		out.customPlanning += time.Duration(custom) * took
		out.firstPlans += took
	}
	return out
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
