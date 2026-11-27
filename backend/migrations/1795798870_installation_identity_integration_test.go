// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

const (
	beforeInstallationIdentity = "1795777900"
	installationIdentity       = "1795798870"
)

func migrateToBeforeInstallationIdentity(t *testing.T) (context.Context, *pgx.Conn, dbmigrate.Namespace) {
	t.Helper()
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()
	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, beforeInstallationIdentity)); err != nil {
		t.Fatalf("up to the migration before: %v", err)
	}
	return ctx, conn, namespaceThroughVersion(t, core, installationIdentity)
}

// An installation with an archived predecessor keeps the live company's id,
// the first company's creation time and the archived count, and a rollback
// removes only those copies.
func TestInstallationIdentityMovesIntoInstallationState(t *testing.T) {
	ctx, conn, through := migrateToBeforeInstallationIdentity(t)
	for _, seed := range []string{
		`INSERT INTO workspace (id, created_at, archived_at) VALUES
		 ('00000000-0000-7000-8000-000000000001', '2025-01-02T03:04:05Z', '2025-06-01T00:00:00Z')`,
		`INSERT INTO workspace (id, created_at) VALUES ('00000000-0000-7000-8000-000000000002', '2025-06-01T00:00:00Z')`,
	} {
		if _, err := conn.Exec(ctx, seed); err != nil {
			t.Fatalf("seeding %s: %v", seed, err)
		}
	}
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	state := `SELECT value::text FROM installation_state WHERE key = `
	for _, c := range []struct{ name, query, want string }{
		{"the stored-key id", state + `'stored_key_workspace'`, `{"id": "00000000-0000-7000-8000-000000000002"}`},
		{"the trial start", `SELECT extract(epoch FROM (value->>'started_at')::timestamptz)::text FROM installation_state WHERE key = 'installation_trial'`, "1735787045.000000"},
		{"the archived count", state + `'archived_predecessor'`, `{"archived_companies": 1}`},
	} {
		if got := scalar(t, conn, c.query); got != c.want {
			t.Errorf("%s is %s, want %s", c.name, got, c.want)
		}
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("rolling back: %v", err)
	}
	if got := scalar(t, conn, `SELECT count(*)::text FROM installation_state WHERE key IN ('stored_key_workspace', 'installation_trial', 'archived_predecessor')`); got != "0" {
		t.Errorf("after the rollback %s copies remain, want 0", got)
	}
	if got := scalar(t, conn, `SELECT count(*)::text FROM workspace`); got != "2" {
		t.Errorf("the rollback left %s workspace rows, want 2", got)
	}
}

// An installation with no company yet writes nothing, and a second run of the
// same statements changes nothing.
func TestInstallationIdentityOnAnEmptyInstallationWritesNothing(t *testing.T) {
	ctx, conn, through := migrateToBeforeInstallationIdentity(t)
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	if got := scalar(t, conn, `SELECT count(*)::text FROM installation_state WHERE key IN ('stored_key_workspace', 'installation_trial', 'archived_predecessor')`); got != "0" {
		t.Errorf("an empty installation gained %s rows, want 0", got)
	}
}

// Two live companies cannot run today, so the upgrade stops instead of
// choosing whose stored keys survive.
func TestInstallationIdentityRefusesTwoLiveCompanies(t *testing.T) {
	ctx, conn, through := migrateToBeforeInstallationIdentity(t)
	if _, err := conn.Exec(ctx, `INSERT INTO workspace (id) VALUES
		('00000000-0000-7000-8000-000000000001'), ('00000000-0000-7000-8000-000000000002')`); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, through); err == nil {
		t.Fatal("two live workspaces migrated, want the upgrade refused")
	}
}
