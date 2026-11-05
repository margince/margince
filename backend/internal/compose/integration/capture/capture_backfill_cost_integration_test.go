// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestHistoryImportReportsItsDatabaseWork(t *testing.T) {
	count := 16
	if value := os.Getenv("MARGINCE_IMPORT_BENCH_MESSAGES"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100000 {
			t.Fatal("MARGINCE_IMPORT_BENCH_MESSAGES must be between 1 and 100000")
		}
		count = parsed
	}
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	page := &mailPageConnector{nextToken: "next", sent: make(map[string]bool, count)}
	for i := range count {
		messageID := fmt.Sprintf("cost-%d@myco.example", i)
		page.raws = append(page.raws, email(captureOwner, "", fmt.Sprintf("cost-%d@globex.example", i), messageID, ""))
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
	if err != nil || done || completed {
		t.Fatalf("import: done=%v completed=%v error=%v", done, completed, err)
	}
	contacts, _ := readBackfillYieldColumns(t, e, run.ID)
	ledger, _ := readCreationLedger(t, e, run.ID)
	if contacts != count || ledger != count {
		t.Fatalf("contacts=%d ledger=%d want=%d", contacts, ledger, count)
	}

	page.raws, page.nextToken = nil, ""
	done, completed, _, err = registry.RunBackfillStep(ctx, run.ID)
	if err != nil || !done || !completed {
		t.Fatalf("completion: done=%v completed=%v error=%v", done, completed, err)
	}
	cost := meter.Cost()
	if cost.Statements == 0 || cost.Acquires == 0 {
		t.Fatalf("capture was not metered: %+v", cost)
	}
	t.Logf("messages=%d statements=%d acquires=%d db_time=%s pool_wait=%s", count, cost.Statements, cost.Acquires, cost.DBTime, cost.PoolWait)
}
