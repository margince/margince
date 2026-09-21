// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The one down-migration test this repo's generic reversal suite cannot
// give for free: TestMigrations_applyReverseReapply reverses every
// migration in one pass, but seedLedgerRowsForReversal seeds only the
// append-only ledgers, never a capture_connection row — so it could not
// have caught 1788873520's own down-migration refusing a real
// connect-then-disconnect lifecycle. This test seeds exactly that lifecycle
// and proves the down-migration survives it.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// namespaceThroughVersion returns core truncated so wantVersion is its LAST
// migration — what lets dbmigrate.Down(..., 1), which only targets "one step
// from the newest in the list I hand it", revert THIS migration specifically
// regardless of whatever has landed above it in the real core by the time
// this test runs. The DB itself is still migrated with the untruncated core
// (see Up below), so this only narrows what Down is told to walk, never what
// Up applied.
func namespaceThroughVersion(t *testing.T, core dbmigrate.Namespace, wantVersion string) dbmigrate.Namespace {
	t.Helper()
	for i, m := range core.Migrations {
		if m.Version == wantVersion {
			return dbmigrate.Namespace{Name: core.Name, Migrations: core.Migrations[:i+1]}
		}
	}
	t.Fatalf("no core migration carries version %s — was it renamed without updating this test?", wantVersion)
	return dbmigrate.Namespace{}
}

// seedAppUserForConnection seeds the row capture_connection.user_id's
// foreign key requires — a real app_user, not a bare UUID.
func seedAppUserForConnection(t *testing.T, conn *pgx.Conn, email string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(context.Background(), `
		INSERT INTO app_user (email, display_name) VALUES ($1, 'Fixture User') RETURNING id`,
		email).Scan(&id); err != nil {
		t.Fatalf("seeding app_user %s: %v", email, err)
	}
	return id
}

func TestTheTestMailboxDownMigrationSurvivesADisconnectedConnection(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("up: %v", err)
	}

	userID := seedAppUserForConnection(t, conn, "disconnected-fixture@test.example")

	// The state a real connect-then-disconnect leaves: Registry.Connect
	// wrote this row with a sealed credential_ref, and Disconnect
	// (withdrawConnection) flips status to 'disconnected' and clears
	// credential_ref to NULL — it does not delete the row.
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_connection (provider, user_id, status, credential_ref)
		VALUES ('test_mailbox', $1, 'disconnected', NULL)`, userID); err != nil {
		t.Fatalf("seeding a disconnected test_mailbox connection: %v", err)
	}

	if _, err := dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1788873520"), 1); err != nil {
		t.Fatalf("down: %v — a disconnected test_mailbox connection must not block rolling this migration back", err)
	}

	var stillPresent bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM capture_connection WHERE provider = 'test_mailbox')`).
		Scan(&stillPresent); err != nil {
		t.Fatalf("checking for a surviving test_mailbox row: %v", err)
	}
	if stillPresent {
		t.Error("a test_mailbox connection row survived the down-migration — the narrower CHECK this migration restores no longer admits it")
	}
}

// A LIVE credential must still refuse the rollback — the down-migration's
// safety guarantee (never strand a sealed secret) has to survive right next
// to the fix that lets a fully-disconnected row through.
func TestTheTestMailboxDownMigrationStillRefusesALiveConnection(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("up: %v", err)
	}

	userID := seedAppUserForConnection(t, conn, "live-fixture@test.example")
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_connection (provider, user_id, status, credential_ref)
		VALUES ('test_mailbox', $1, 'connected', 'vault-ref-still-live')`, userID); err != nil {
		t.Fatalf("seeding a live test_mailbox connection: %v", err)
	}

	_, err = dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1788873520"), 1)
	if err == nil {
		t.Fatal("down succeeded against a live test_mailbox credential — it must refuse rather than strand the sealed secret")
	}
	if !strings.Contains(err.Error(), "cannot roll back") {
		t.Fatalf("down failed with %v, want the DO block's own refusal (\"cannot roll back\") — "+
			"an unrelated failure (syntax, lock timeout) would pass this test for the wrong reason", err)
	}
}
