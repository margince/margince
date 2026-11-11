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
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestHistoryImportReportsItsDatabaseWork(t *testing.T) {
	count := historyMeasurementSize(t, "MARGINCE_IMPORT_BENCH_MESSAGES", 16, 100000)
	parties := historyMeasurementSize(t, "MARGINCE_IMPORT_BENCH_CONTACTS", count, count)
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	pool := e.Pool
	if os.Getenv("MARGINCE_IMPORT_BENCH_SQL") != "" {
		pool = profileHistoryPool(e.Admin(), t, pool)
	}
	// The tally write and the ownership check tick on wall time. A slow runner
	// would add work this report counts, so both are held off, as below.
	registry := compose.NewCaptureRegistry(pool, newTestKeyvault(t, e), compose.CaptureConfig{}).
		WithProgressPacing(24 * time.Hour).WithOwnershipWatch(24 * time.Hour)
	page := &mailPageConnector{nextToken: "next", sent: make(map[string]bool, count)}
	for i := range count {
		messageID := fmt.Sprintf("cost-%d@myco.example", i)
		page.raws = append(page.raws, email(captureOwner, "", fmt.Sprintf("cost-%d@globex.example", i%parties), messageID, ""))
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
	segmentSize, captured := max(1, min(1000, count/10)), 0
	previous := meter.Cost()
	page.afterMessage = func() {
		captured++
		if captured%segmentSize == 0 || captured == count {
			current := meter.Cost()
			segment := current.Minus(previous)
			t.Logf("segment_end=%d statements=%d db_time=%s pool_wait=%s", captured, segment.Statements, segment.DBTime, segment.PoolWait)
			previous = current
		}
	}
	done, completed, _, err := registry.RunBackfillStep(ctx, run.ID)
	if err != nil || done || completed {
		t.Fatalf("import: done=%v completed=%v error=%v", done, completed, err)
	}
	contacts, _ := readBackfillYieldColumns(t, e, run.ID)
	ledger, _ := readCreationLedger(t, e, run.ID)
	if contacts != parties || ledger != parties || captured != count {
		t.Fatalf("contacts=%d ledger=%d want=%d; captured=%d want=%d", contacts, ledger, parties, captured, count)
	}

	page.raws, page.nextToken = nil, ""
	done, completed, retry, err := registry.RunBackfillStep(ctx, run.ID)
	if err != nil || !done || !completed {
		t.Fatalf("completion: done=%v completed=%v retry=%v error=%v", done, completed, retry, err)
	}
	cost := meter.Cost()
	if cost.Statements == 0 || cost.Acquires == 0 {
		t.Fatalf("capture was not metered: %+v", cost)
	}
	t.Logf("messages=%d statements=%d exchanges=%d transactions=%d acquires=%d db_time=%s pool_wait=%s", count, cost.Statements, cost.Exchanges, cost.Transactions, cost.Acquires, cost.DBTime, cost.PoolWait)
	t.Logf("contacts=%d", parties)
}

// The budget is per message. The mailbox holds ten exchanges with each of
// twenty correspondents, half sent and half received. So it fails a change
// that adds round trips to every message. Lower it when a change removes
// some; never raise it to make a change pass.
// Thread membership and mailbox generation are checked before each message write.
// It includes one statement per filed message that names the linked contact as
// the sender when the stated address proves them (stampProvenSenders).
const (
	historyExchangesPerMessage    = 187
	historyTransactionsPerMessage = 6.15
)

func TestHistoryImportStaysWithinItsRoundTripBudget(t *testing.T) {
	const count, parties = 200, 20
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	pool := e.Pool
	if os.Getenv("MARGINCE_IMPORT_BENCH_SQL") != "" {
		pool = profileHistoryPool(e.Admin(), t, pool)
	}
	// The live tally write and the page's ownership check both run on wall
	// time, so their count follows the runner's speed rather than the code.
	// Holding both off for the page keeps the budget about each message.
	registry := compose.NewCaptureRegistry(pool, newTestKeyvault(t, e), compose.CaptureConfig{}).
		WithProgressPacing(24 * time.Hour).WithOwnershipWatch(24 * time.Hour)
	page := &mailPageConnector{nextToken: "next", sent: make(map[string]bool, count)}
	for i := range count {
		messageID, party := fmt.Sprintf("budget-%d@myco.example", i), fmt.Sprintf("budget-%d@globex.example", (i/2)%parties)
		if i%2 == 1 {
			page.raws = append(page.raws, email(party, "", captureOwner, messageID, ""))
			continue
		}
		page.raws = append(page.raws, email(captureOwner, "", party, messageID, ""))
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
	if done, completed, _, err := registry.RunBackfillStep(ctx, run.ID); err != nil || done || completed {
		t.Fatalf("import: done=%v completed=%v error=%v", done, completed, err)
	}
	cost := meter.Cost()
	if contacts, _ := readBackfillYieldColumns(t, e, run.ID); contacts != parties {
		t.Fatalf("contacts=%d want %d", contacts, parties)
	}
	exchanges, transactions := float64(cost.Exchanges)/count, float64(cost.Transactions)/count
	t.Logf("per message: exchanges=%.1f transactions=%.2f statements=%.1f", exchanges, transactions, float64(cost.Statements)/count)
	if exchanges > historyExchangesPerMessage || transactions > historyTransactionsPerMessage {
		t.Fatalf("per message: %.1f exchanges (budget %d), %.2f transactions (budget %g)", exchanges, historyExchangesPerMessage, transactions, historyTransactionsPerMessage)
	}
}

func historyMeasurementSize(t *testing.T, name string, fallback, limit int) int {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > limit {
		t.Fatalf("%s must be between 1 and %d", name, limit)
	}
	return parsed
}
