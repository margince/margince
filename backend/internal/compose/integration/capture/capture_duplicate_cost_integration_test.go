// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A second mailbox delivering mail that is already captured costs this many
// database round trips per message (107.9 when set). Lower it when a change
// removes some; never raise it to make a change pass.
const duplicateExchangesPerMessage = 110

// The content tables a proven duplicate must not grow. The import row and the
// participant row are this seat's claim on the message; everything else is the
// message itself, which is stored once.
var contentTablesADuplicateLeavesAlone = []string{"raw_capture", "activity", "attachment", "embedding"}

func TestASecondMailboxDeliveringKnownMailStoresNoContentAndStaysWithinItsBudget(t *testing.T) {
	const count = 40
	env := newCaptureEnv(t)
	e := env.e
	seedCaptureRoleFor(t, e, e.Rep3)
	raws := make([][]byte, 0, count)
	for i := range count {
		raws = append(raws, email(fmt.Sprintf("dup-%d@globex.example", i%8), "", captureOwner+", "+secondSeatAddress, fmt.Sprintf("dup-%d@globex.example", i), ""))
	}
	env.sync(t, raws...)
	before := tableCounts(t, e, contentTablesADuplicateLeavesAlone)
	imports := tableCounts(t, e, []string{"capture_import"})["capture_import"]

	conn := &mailBatchConnector{accountLabel: secondSeatAddress}
	pool := e.Pool
	if os.Getenv("MARGINCE_IMPORT_BENCH_SQL") != "" {
		pool = profileHistoryPool(e.Admin(), t, pool)
	}
	registry := compose.NewCaptureRegistry(pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	registry.Register(conn)
	connID, err := registry.Connect(humanWithScopes(e, e.Rep3, []principal.Scope{principal.ScopeRead}), "gmail", connector.Auth("refresh"))
	if err != nil {
		t.Fatalf("connecting the second mailbox: %v", err)
	}
	conn.raws = raws
	ctx, meter := database.WithStatementMeter(principal.WithWorkspaceID(context.Background(), e.WS))
	if err := registry.SyncOnce(ctx, connID); err != nil {
		t.Fatalf("second mailbox SyncOnce: %v", err)
	}
	cost := meter.Cost()
	exchanges := float64(cost.Exchanges) / count
	t.Logf("per duplicate: exchanges=%.1f transactions=%.2f statements=%.1f", exchanges, float64(cost.Transactions)/count, float64(cost.Statements)/count)

	after := tableCounts(t, e, contentTablesADuplicateLeavesAlone)
	for _, table := range contentTablesADuplicateLeavesAlone {
		if after[table] != before[table] {
			t.Errorf("%s grew from %d to %d rows; a proven duplicate stores no content", table, before[table], after[table])
		}
	}
	if got := tableCounts(t, e, []string{"capture_import"})["capture_import"]; got != imports+count {
		t.Fatalf("capture_import has %d rows, want %d: every delivered copy is one import row", got, imports+count)
	}
	if exchanges > duplicateExchangesPerMessage {
		t.Fatalf("a duplicate arrival costs %.1f round trips (budget %d)", exchanges, duplicateExchangesPerMessage)
	}
}

func tableCounts(t *testing.T, e *integration.SearchEnv, tables []string) map[string]int {
	t.Helper()
	counts := make(map[string]int, len(tables))
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, table := range tables {
			var n int
			if err := tx.QueryRow(context.Background(), `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&n); err != nil {
				return err
			}
			counts[table] = n
		}
		return nil
	})
	if err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	return counts
}
