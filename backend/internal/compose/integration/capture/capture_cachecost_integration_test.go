// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestWorkerCacheReportsHistoryPreparationCost(t *testing.T) {
	costs := make(map[int]database.StatementCost)
	for _, capacity := range []int{64, 128, 256, 512} {
		t.Run(fmt.Sprintf("cache-%d", capacity), func(t *testing.T) {
			e := integration.SetupSearch(t)
			seedCaptureRole(t, e)
			cfg, err := database.WorkerPoolConfig(os.Getenv("MARGINCE_TEST_APP_DSN"), compose.WorkerQueueConcurrency(), 8, capacity)
			if err != nil {
				t.Fatal(err)
			}
			pool, err := testdb.OwnPoolFromConfig(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(pool.Close)
			registry := compose.NewCaptureRegistry(pool, newTestKeyvault(t, e), compose.CaptureConfig{})
			page := &mailPageConnector{sent: make(map[string]bool)}
			const count = 16
			for i := range count {
				messageID := fmt.Sprintf("cache-%d@myco.example", i)
				page.raws = append(page.raws, email(captureOwner, "", fmt.Sprintf("cache-%d@globex.example", i), messageID, ""))
				page.sent[messageID] = true
			}
			registry.Register(page)
			actor := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
			if _, err := registry.Connect(actor, "gmail", connector.Auth("refresh")); err != nil {
				t.Fatal(err)
			}
			run, err := registry.StartBackfill(actor, "gmail", ids.From[ids.UserKind](e.Rep1), 6, connector.BackfillEstimate{Messages: count}, enqueueNothing)
			if err != nil {
				t.Fatal(err)
			}
			ctx, meter := database.WithStatementMeter(principal.WithWorkspaceID(context.Background(), e.WS))
			done, completed, _, err := registry.RunBackfillStep(ctx, run.ID)
			if err != nil || !done || !completed {
				t.Fatalf("import: done=%v completed=%v err=%v", done, completed, err)
			}
			contacts, _ := readBackfillYieldColumns(t, e, run.ID)
			if contacts != count {
				t.Fatalf("contacts=%d want=%d", contacts, count)
			}
			cost := meter.Cost()
			costs[capacity] = cost
			if cost.Prepares == 0 || cost.Statements == 0 {
				t.Fatalf("unmetered import: %+v", cost)
			}
			t.Logf("cache=%d messages=%d prepares=%d statements=%d acquires=%d db_time=%s pool_wait=%s", capacity, count, cost.Prepares, cost.Statements, cost.Acquires, cost.DBTime, cost.PoolWait)
		})
	}
	if costs[256].Prepares > 2*costs[512].Prepares {
		t.Fatalf("bounded cache thrashes: prepares=%d full-cache prepares=%d", costs[256].Prepares, costs[512].Prepares)
	}
}
