// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const installationNameMigration = "core/1791994619_the_installation_carries_its_companys_name.up.sql"

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
	if n := migrationAudits(ctx, t, conn); n != 0 {
		t.Errorf("an unchanged name was audited %d times", n)
	}
}

func migrationAudits(ctx context.Context, t *testing.T, conn *pgx.Conn) int {
	t.Helper()
	var audits int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE actor_id = 'migration'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	return audits
}

func TestAnAnchorNameTheSettingRefusesLeavesTheConfiguredName(t *testing.T) {
	for label, name := range map[string]string{
		"blank":    "   ",
		"too long": strings.Repeat("a", 201),
	} {
		t.Run(label, func(t *testing.T) {
			dsn, _ := dsns(t)
			conn := connect(t, dsn)
			headSchema(t, conn)
			ctx := context.Background()
			seedInstallationName(ctx, t, conn, "Configured GmbH")
			if _, err := conn.Exec(ctx, `
				INSERT INTO company (display_name, source, captured_by, is_anchor)
				VALUES ($1, 'manual', 'human:test', true)`, name); err != nil {
				t.Fatalf("seeding the anchor company: %v", err)
			}

			runFile(ctx, t, conn, installationNameMigration)

			if got := storedInstallationName(ctx, t, conn); got != "Configured GmbH" {
				t.Errorf("installation name = %q, want the configured name kept", got)
			}
			if n := migrationAudits(ctx, t, conn); n != 0 {
				t.Errorf("a name left alone was audited %d times", n)
			}
		})
	}
}

func TestANameTheSettingAlreadyHoldsIsNotAudited(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	seedInstallationName(ctx, t, conn, "Profile Name AG")
	if _, err := conn.Exec(ctx, `
		INSERT INTO company (display_name, source, captured_by, is_anchor)
		VALUES ('Profile Name AG', 'manual', 'human:test', true)`); err != nil {
		t.Fatalf("seeding the anchor company: %v", err)
	}

	runFile(ctx, t, conn, installationNameMigration)

	if n := migrationAudits(ctx, t, conn); n != 0 {
		t.Errorf("an unchanged name was audited %d times", n)
	}
}

// With no stored row the setting reads as its registered default, so the
// audit's before image is that default, as SetRawTx writes it.
func TestAFirstStoredNameIsAuditedFromTheDefault(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx,
		`INSERT INTO workspace (id) SELECT uuidv7() WHERE NOT EXISTS (SELECT 1 FROM workspace)`); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO company (display_name, source, captured_by, is_anchor)
		VALUES ('Profile Name AG', 'manual', 'human:test', true)`); err != nil {
		t.Fatalf("seeding the anchor company: %v", err)
	}

	runFile(ctx, t, conn, installationNameMigration)

	var before string
	var entity, workspace string
	if err := conn.QueryRow(ctx, `
		SELECT before -> 'installation.name' #>> '{}', entity_id::text, (SELECT id::text FROM workspace)
		  FROM audit_log WHERE actor_id = 'migration'`).Scan(&before, &entity, &workspace); err != nil {
		t.Fatalf("the first stored name left no audit row: %v", err)
	}
	if before != "" {
		t.Errorf("audit before = %q, want the registered default \"\"", before)
	}
	if entity != workspace {
		t.Errorf("audit names entity %s, want the installation's workspace %s", entity, workspace)
	}
}
