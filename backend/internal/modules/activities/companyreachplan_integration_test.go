// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The plan behind the account walk. A walk that reads every activity of the
// installation answers correctly and slowly, so only the plan can show it.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheAccountReachWalkNeverScansEveryActivity(t *testing.T) {
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded account: %v", err)
		}
	}()
	busy := ids.NewV7()
	seedBusyAccount(ctx, t, tx, busy)

	// Some account mail reaches it only through an employee on the message.
	if _, err := tx.Exec(ctx, `ALTER TABLE activity_participant DISABLE TRIGGER USER;
		INSERT INTO activity_participant (activity_id, contact_id, role)
		SELECT id, contact_id, 'cc' FROM seeded_mail WHERE g % 2 = 0;
		DELETE FROM activity_link l USING seeded_mail m
		WHERE l.activity_id = m.id AND m.g % 10 = 2;
		ANALYZE activity_participant; ANALYZE activity_link`); err != nil {
		t.Fatal(err)
	}

	walks := []struct {
		name string
		sql  string
		rows float64
	}{
		{"filed", storekit.CompanyFiledActivityReach("$1"), 4500},
		{"walked", storekit.CompanyActivityReach("$1"), 5000},
	}
	for _, walk := range walks {
		name := walk.name
		statement := "SELECT reach.activity_id FROM (" + walk.sql + ") reach"
		var raw string
		if err := tx.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+statement, busy).Scan(&raw); err != nil {
			t.Fatalf("planning the %s walk: %v", name, err)
		}
		var plans []struct {
			Plan          planNode `json:"Plan"`           //nolint:tagliatelle // fixed by the server's plan format
			ExecutionTime float64  `json:"Execution Time"` //nolint:tagliatelle // fixed by the server's plan format
		}
		if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
			t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
		}
		if got := plans[0].Plan.ActualRows; got != walk.rows {
			t.Fatalf("the %s walk returned %.0f activities, want %.0f", name, got, walk.rows)
		}
		var activityScans int
		plans[0].Plan.walk(func(n planNode) {
			if n.RelationName == "activity" && n.NodeType == "Seq Scan" {
				activityScans++
				t.Logf("%s walk: sequential scan of activity returned %.0f rows after removing %.0f", name, n.ActualRows, n.RemovedByFilter)
			}
		})
		t.Logf("%s walk: %.1f ms", name, plans[0].ExecutionTime)
		if activityScans != 0 {
			t.Errorf("the %s walk scans activity sequentially %d times: it reads every activity of the installation to find the account's own", name, activityScans)
		}
	}
}

func TestCollapsedCompanyHistoryDoesNotRescanTheActivityTableForEachGroup(t *testing.T) {
	e := setupPromises(t)
	ctx := e.as()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded account: %v", err)
		}
	}()
	busy := ids.NewV7()
	seedBusyAccount(ctx, t, tx, busy)
	// Keep enough groups to fill the page while most activity belongs elsewhere.
	if _, err := tx.Exec(ctx, `DELETE FROM activity_link l USING seeded_mail m
		WHERE l.activity_id = m.id AND m.company_id = (SELECT id FROM seeded_busy) AND m.g > 87;
		ANALYZE activity_link`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL plan_cache_mode = force_generic_plan; SET LOCAL jit = off`); err != nil {
		t.Fatal(err)
	}
	const limit = 25
	in := ListActivitiesInput{EntityType: new("company"), EntityID: &busy, CollapseSeatCopies: true}
	page, err := timelinePageSQL(ctx, in, limit)
	if err != nil {
		t.Fatal(err)
	}
	args := waitingListArgs(ctx, in, page.args)
	if _, err := tx.Exec(ctx, page.sql, append([]any{pgx.QueryExecModeCacheStatement}, args...)...); err != nil {
		t.Fatal(err)
	}
	raw, err := testdb.ExplainCachedStatement(ctx, tx, page.sql, args)
	if err != nil {
		t.Fatal(err)
	}
	plan := decodePlan(t, string(raw))
	if plan.ActualRows != limit+1 {
		t.Fatalf("history returned %.0f rows, want a full page and its lookahead", plan.ActualRows)
	}
	var grouped bool
	plan.walk(func(n planNode) {
		if !grouped && (n.Alias == "copies" || n.NodeType == "Unique") && n.ActualRows > limit {
			grouped = true
			if n.PlanRows < limit {
				t.Errorf("the join estimates %.0f representatives for %.0f groups, fewer than one page", n.PlanRows, n.ActualRows)
			}
		}
		if n.NodeType == "Seq Scan" && n.RelationName == "activity" && n.Alias == "a" && n.ActualLoops > 1 {
			t.Errorf("hydrating the collapsed history scanned activity sequentially %.0f times", n.ActualLoops)
		}
	})
	if !grouped {
		t.Error("plan does not show the grouping that feeds activity hydration")
	}
}
