// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const installationNameMigration = "core/1791968194_the_installation_carries_its_companys_name.up.sql"

func seedInstallationName(ctx context.Context, t *testing.T, conn *pgx.Conn, name string) {
	t.Helper()
	if _, err := conn.Exec(ctx,
		`INSERT INTO workspace (id) SELECT uuidv7() WHERE NOT EXISTS (SELECT 1 FROM workspace)`); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO setting (key, value) VALUES ('installation.name', to_jsonb($1::text))`, name); err != nil {
		t.Fatalf("seeding the configured name: %v", err)
	}
}

func storedInstallationName(ctx context.Context, t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	var name string
	if err := conn.QueryRow(ctx,
		`SELECT value #>> '{}' FROM setting WHERE key = 'installation.name'`).Scan(&name); err != nil {
		t.Fatalf("reading the installation name: %v", err)
	}
	return name
}

func TestTheInstallationTakesTheNameItsCompanyProfileSaved(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	seedInstallationName(ctx, t, conn, "Configured GmbH")
	if _, err := conn.Exec(ctx, `
		INSERT INTO company (display_name, source, captured_by, is_anchor)
		VALUES ('Profile Name AG', 'manual', 'human:test', true)`); err != nil {
		t.Fatalf("seeding the anchor company: %v", err)
	}

	runFile(ctx, t, conn, installationNameMigration)

	if got := storedInstallationName(ctx, t, conn); got != "Profile Name AG" {
		t.Errorf("installation name = %q, want the anchor's display name %q", got, "Profile Name AG")
	}
	var before, after string
	if err := conn.QueryRow(ctx, `
		SELECT before ->> 'installation.name', after ->> 'installation.name'
		  FROM audit_log
		 WHERE entity_type = 'installation_settings' AND actor_id = 'migration'`).Scan(&before, &after); err != nil {
		t.Fatalf("the copy left no audit row: %v", err)
	}
	if before != "Configured GmbH" || after != "Profile Name AG" {
		t.Errorf("audit says %q -> %q, want the configured name to the profile's", before, after)
	}
}

func TestAnInstallationWithNoCompanyProfileKeepsItsConfiguredName(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	seedInstallationName(ctx, t, conn, "Configured GmbH")
	if _, err := conn.Exec(ctx, `
		INSERT INTO company (display_name, source, captured_by)
		VALUES ('A Customer Ltd', 'manual', 'human:test')`); err != nil {
		t.Fatalf("seeding a customer company: %v", err)
	}

	runFile(ctx, t, conn, installationNameMigration)

	if got := storedInstallationName(ctx, t, conn); got != "Configured GmbH" {
		t.Errorf("installation name = %q, want the configured name kept with no anchor", got)
	}
	var audits int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE actor_id = 'migration'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Errorf("an unchanged name was audited %d times", audits)
	}
}
