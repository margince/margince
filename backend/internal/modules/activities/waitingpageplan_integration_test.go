// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

var waitingPlanProbeFromSQL = `FROM activity a
 WHERE a.kind IN ('email', 'message') AND a.direction = 'inbound' AND a.archived_at IS NULL
   AND (SELECT count(*) FROM activity_participant probe WHERE probe.activity_id = a.id) >= 0
   AND ` + withinWaitingHorizonSQL("$%[1]d::timestamptz", "%[5]d")

var waitingPlanHeldMailSQL = `WITH reminders AS (
 INSERT INTO activity (kind, subject, occurred_at, source, captured_by, source_system, source_activity_id)
 SELECT 'task', 'Reply', occurred_at, 'manual', 'human:seed', '` + EmailRequestTaskSource + `', id
 FROM activity WHERE direction = 'inbound' ORDER BY occurred_at, id LIMIT 2 RETURNING id)
 INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
 SELECT 'human', 'seed', 'create', 'activity', id FROM reminders;`

func waitingPlanWork(plan planNode) (judged, read float64) {
	plan.walk(func(n planNode) {
		if planCopySuffix.ReplaceAllString(n.Alias, "") == "probe" {
			judged += n.ActualLoops
		}
		if n.RelationName == "activity" && planCopySuffix.ReplaceAllString(n.Alias, "") == "a" {
			read += (n.ActualRows + n.RemovedByFilter) * n.ActualLoops
		}
	})
	return judged, read
}

func TestTheWaitingPageSeeksPastACursorInsideTimestampTies(t *testing.T) {
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}()
	seedMailbox(ctx, t, tx, busyMailbox)
	if _, err := tx.Exec(ctx, `UPDATE activity SET occurred_at = (
	 SELECT max(occurred_at) FROM activity WHERE direction = 'inbound') WHERE direction = 'inbound'`); err != nil {
		t.Fatal(err)
	}
	var cursor WaitingPosition
	if err := tx.QueryRow(ctx, `SELECT occurred_at, id FROM activity WHERE direction = 'inbound'
	 ORDER BY occurred_at DESC, id DESC OFFSET 750 LIMIT 1`).Scan(&cursor.At, &cursor.ID); err != nil {
		t.Fatal(err)
	}
	const pageCap = 3
	args := []any{clocktest.Now(t)}
	instant := len(args)
	arg := func(value any) int {
		args = append(args, value)
		return len(args)
	}
	limit := waitingLimitSQL(arg, pageCap) + "::int"
	keyset := olderThan(cursor, arg)
	statement := fmt.Sprintf(strings.ReplaceAll(waitingCappedArmsSQL, waitingEligibleFromSQL,
		waitingPlanProbeFromSQL+keyset), instant, nil, nil, limit, 30)
	if _, err := tx.Exec(ctx, "SET LOCAL plan_cache_mode = force_generic_plan; SET LOCAL max_parallel_workers_per_gather = 0"); err != nil {
		t.Fatal(err)
	}
	for _, analyzed := range []bool{false, true} {
		if analyzed {
			if _, err := tx.Exec(ctx, "ANALYZE activity, activity_participant"); err != nil {
				t.Fatal(err)
			}
		} else {
			pinNeverAnalyzed(ctx, t, tx)
		}
		for _, incremental := range []string{"on", "off"} {
			if _, err := tx.Exec(ctx, "SET LOCAL enable_incremental_sort = "+incremental); err != nil {
				t.Fatal(err)
			}
			var raw string
			if err := tx.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+statement, args...).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			plan := decodePlan(t, raw)
			judged, read := waitingPlanWork(plan)
			if plan.ActualRows != pageCap || judged > 2*pageCap || read > 2*pageCap {
				t.Errorf("analyzed=%t incremental=%s: returned %v, judged %v, read %v for cap %d", analyzed, incremental, plan.ActualRows, judged, read, pageCap)
			}
		}
	}
}
