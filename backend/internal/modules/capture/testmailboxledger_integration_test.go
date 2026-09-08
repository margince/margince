// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// testMailboxLedgerDB builds the ledger over a real schema, mirroring
// traceReadWorkspace's own setup — this table needs no workspace row, since
// it carries no workspace_id, but the pool/connection setup is identical.
func testMailboxLedgerDB(t *testing.T) (context.Context, *capture.TestMailboxLedger) {
	t.Helper()
	_, pool := setupCaptureDB(t)
	ws := ids.NewV7()
	db := database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))
	return context.Background(), capture.NewTestMailboxLedger(db)
}

func TestTestMailboxLedgerRecordsAndEchoesOnce(t *testing.T) {
	ctx, ledger := testMailboxLedgerDB(t)
	userID := ids.NewV7()

	if err := ledger.RecordSent(ctx, userID, "abc123@test.example", []string{"buyer@example.com"}, "Hello"); err != nil {
		t.Fatalf("RecordSent: %v", err)
	}

	unechoed, err := ledger.Unechoed(ctx, userID)
	if err != nil {
		t.Fatalf("Unechoed: %v", err)
	}
	if len(unechoed) != 1 || unechoed[0].MessageID != "abc123@test.example" {
		t.Fatalf("Unechoed = %+v, want one row for abc123@test.example", unechoed)
	}
	if len(unechoed[0].To) != 1 || unechoed[0].To[0] != "buyer@example.com" {
		t.Fatalf("Unechoed[0].To = %v, want [buyer@example.com]", unechoed[0].To)
	}

	if err := ledger.MarkEchoed(ctx, unechoed[0].ID); err != nil {
		t.Fatalf("MarkEchoed: %v", err)
	}

	unechoed, err = ledger.Unechoed(ctx, userID)
	if err != nil {
		t.Fatalf("Unechoed after mark: %v", err)
	}
	if len(unechoed) != 0 {
		t.Fatalf("Unechoed after MarkEchoed = %+v, want none", unechoed)
	}
}

// A second seat's sends must never surface in the first's echo list — the
// ledger has no workspace_id, so user_id is the only boundary it has, and it
// has to hold on its own.
func TestTestMailboxLedgerIsolatesByUser(t *testing.T) {
	ctx, ledger := testMailboxLedgerDB(t)
	me, other := ids.NewV7(), ids.NewV7()

	if err := ledger.RecordSent(ctx, me, "mine@test.example", []string{"buyer@example.com"}, "Mine"); err != nil {
		t.Fatalf("RecordSent(me): %v", err)
	}
	if err := ledger.RecordSent(ctx, other, "theirs@test.example", []string{"buyer@example.com"}, "Theirs"); err != nil {
		t.Fatalf("RecordSent(other): %v", err)
	}

	unechoed, err := ledger.Unechoed(ctx, me)
	if err != nil {
		t.Fatalf("Unechoed(me): %v", err)
	}
	if len(unechoed) != 1 || unechoed[0].MessageID != "mine@test.example" {
		t.Fatalf("Unechoed(me) = %+v, want only mine@test.example", unechoed)
	}
}
