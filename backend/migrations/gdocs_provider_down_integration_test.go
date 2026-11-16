// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func TestMeetingNotesRollbackRestoresTheValidatedProviderVocabulary(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"1794412054", "1794379079"} {
		t.Run(version, func(t *testing.T) { assertMeetingNotesRollback(t, version) })
	}
}

func assertMeetingNotesRollback(t *testing.T, version string) {
	t.Helper()
	owner, _ := dsns(t)
	conn := connect(t, owner)
	resetSchema(t, conn)
	ctx := context.Background()
	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatal(err)
	}
	through := namespaceThroughVersion(t, core, "1794412055")
	through.Renumbered = core.Renumbered
	legacy := through
	legacy.Migrations = slices.Clone(through.Migrations)
	legacy.Renumbered = nil
	for i := range legacy.Migrations {
		migration := &legacy.Migrations[i]
		if migration.Version != "1794412054" {
			continue
		}
		migration.DownSQL = strings.TrimSuffix(migration.DownSQL, ");\n") + ") NOT VALID;\n"
		migration.Version = version
		wantDigest := "7095201fa85775e88f7fa30e3c6e1c33a274f457f9e37118b74a51da14bfddcd"
		if version == "1794379079" {
			wantDigest = "b072a9301b6a7adb4b5cfdc6e97461da09d02a515fbbe3725e681ee393f0e499"
		}
		if dbmigrate.Digest(*migration) != wantDigest {
			t.Fatal("legacy rollback fixture no longer matches the admitted bytes")
		}
	}
	if _, err := dbmigrate.Up(ctx, conn, legacy); err != nil {
		t.Fatal(err)
	}
	wantReconciled := 0
	if version != "1794412054" {
		wantReconciled = 1
	}
	if report, err := dbmigrate.UpWithReport(ctx, conn, through); err != nil || report.Applied != 0 || report.Reconciled != wantReconciled {
		t.Fatalf("corrected source did not admit its identical applied up: %+v %v", report, err)
	}
	user := seedAppUserForConnection(t, conn, "notes-rollback@test.example")
	if _, err := conn.Exec(ctx, `INSERT INTO capture_connection(provider,user_id,status,credential_ref) VALUES('gdocs',$1,'connected','fixture-reference')`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 2); err == nil {
		t.Fatal("rollback discarded a live credential reference")
	}
	if _, err := conn.Exec(ctx, `UPDATE capture_connection SET status='disconnected',credential_ref=NULL WHERE provider='gdocs'`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatal(err)
	}
	var validated bool
	var definition string
	if err := conn.QueryRow(ctx, `SELECT convalidated,pg_get_constraintdef(oid) FROM pg_constraint WHERE conname='capture_connection_provider_check'`).Scan(&validated, &definition); err != nil {
		t.Fatal(err)
	}
	if !validated || strings.Contains(definition, "gdocs") {
		t.Fatalf("rollback vocabulary validated=%t: %s", validated, definition)
	}
	var rows int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM capture_connection WHERE provider='gdocs'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatal("withdrawn notes connection survived rollback")
	}
}
