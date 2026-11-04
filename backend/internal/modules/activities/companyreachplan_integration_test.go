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

	walks := map[string]string{
		"filed":  "SELECT reach.activity_id FROM (" + companyLinkReach("$1") + ") reach",
		"walked": "SELECT reach.activity_id FROM (" + companyReach("$1") + ") reach",
	}
	for name, statement := range walks {
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
		if plans[0].Plan.ActualRows == 0 {
			t.Fatalf("the %s walk returned no activity of the seeded account, so its plan proves nothing", name)
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
