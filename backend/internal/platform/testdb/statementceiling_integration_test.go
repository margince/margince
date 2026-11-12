// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestACappedConnectionHoldsTheCeilingAcrossItsTransactions(t *testing.T) {
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing the connection: %v", err)
		}
	})
	if capped, setting, err := StatementsCapped(ctx, conn); err != nil || capped {
		t.Fatalf("an uncapped connection reads as capped (%q, %v), so the check below proves nothing", setting, err)
	}
	if err := CapStatements(ctx, conn); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if capped, setting, err := StatementsCapped(ctx, conn); err != nil || !capped {
		t.Fatalf("after a transaction the connection's statement_timeout is %q (%v), want %v", setting, err, OwnerStatementCeiling)
	}
}
