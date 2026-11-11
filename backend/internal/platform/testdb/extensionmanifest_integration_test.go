// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

func TestTheCloneProbeChecksEveryComposedExtensionLedger(t *testing.T) {
	ctx := context.Background()
	owner := ownerConn(t)
	existing, err := testExtensionNamespaces()
	if err != nil {
		t.Fatal(err)
	}
	old := dbmigrate.Migration{Version: "0010", Name: "probe", UpSQL: "SELECT 1", DownSQL: "SELECT 1"}
	current := old
	current.Version = "0020"
	ns := dbmigrate.Namespace{Name: "ext_head_probe", Migrations: []dbmigrate.Migration{current}, Renumbered: []dbmigrate.Renumber{{Old: old.Version, New: current.Version, Name: old.Name, OldDigest: dbmigrate.Digest(old)}}}
	table := pgx.Identifier{"schema_migrations_" + ns.Name}.Sanitize()
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS `+table); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	if _, err := dbmigrate.Up(ctx, owner, dbmigrate.Namespace{Name: ns.Name, Migrations: []dbmigrate.Migration{old}}); err != nil {
		t.Fatal(err)
	}
	if reason, err := ledgerAtHead(ctx, owner); err != nil || !strings.Contains(reason, "absent from") {
		t.Fatalf("unlisted extension: %q %v", reason, err)
	}
	body, err := json.Marshal(append(existing, ns))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(extensionManifestEnv, path)
	if reason, err := ledgerAtHead(ctx, owner); err != nil || reason != "" {
		t.Fatalf("declared old stamp: %q %v", reason, err)
	}
	if _, err := owner.Exec(ctx, `UPDATE `+table+` SET content_digest='wrong'`); err != nil {
		t.Fatal(err)
	}
	if reason, err := ledgerAtHead(ctx, owner); err != nil || !strings.Contains(reason, "different content") {
		t.Fatalf("edited extension: %q %v", reason, err)
	}
	if _, err := owner.Exec(ctx, `DELETE FROM `+table); err != nil {
		t.Fatal(err)
	}
	if reason, err := ledgerAtHead(ctx, owner); err != nil || !strings.Contains(reason, "not recorded") {
		t.Fatalf("short extension: %q %v", reason, err)
	}
}

func TestAnAbsentExtensionCensusCannotProveTheCloneAtHead(t *testing.T) {
	t.Setenv(extensionManifestEnv, "")
	if _, err := testExtensionNamespaces(); err == nil {
		t.Fatal("an absent composed migration census was treated as no extensions")
	}
}
