// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

const savedViewNameMigration = "core/1795772279_a_saved_view_name_has_one_live_owner"

func TestSavedViewTwinsKeepTheOldestNameAndSkipExistingSuffixes(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	down, err := os.ReadFile(savedViewNameMigration + ".down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile(savedViewNameMigration + ".up.sql")
	if err != nil {
		t.Fatal(err)
	}
	ns := dbmigrate.Namespace{Name: "saved_view_names_test", Migrations: []dbmigrate.Migration{{Version: "0001", Name: "names", UpSQL: string(down), DownSQL: string(up)}}}
	if _, err := dbmigrate.Up(ctx, conn, ns); err != nil {
		t.Fatal(err)
	}
	owner := seedAppUserForConnection(t, conn, "view-name-twins@example.test")
	seed := func(name, at string) string {
		t.Helper()
		var id string
		err := conn.QueryRow(ctx, `INSERT INTO saved_view (owner_id, resource, name, query, created_at) VALUES ($1, 'contacts', $2, '{}'::jsonb, $3) RETURNING id::text`, owner, name, at).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := seed("Focus", "2026-01-01T00:00:00Z")
	second := seed("focus", "2026-01-02T00:00:00Z")
	held := seed("focus (2)", "2026-01-03T00:00:00Z")
	third := seed("FOCUS", "2026-01-04T00:00:00Z")

	retiredOwner := seedAppUserForConnection(t, conn, "retired-view-owner@example.test")
	var retiredDuplicate string
	for _, at := range []string{"2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"} {
		if err := conn.QueryRow(ctx, `INSERT INTO saved_view (owner_id, resource, name, query, created_at) VALUES ($1, 'contacts', 'Retired focus', '{}'::jsonb, $2) RETURNING id::text`, retiredOwner, at).Scan(&retiredDuplicate); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Exec(ctx, `UPDATE app_user SET status = 'deactivated', removed_at = now() WHERE id = $1`, retiredOwner); err != nil {
		t.Fatal(err)
	}
	applyMigrationFile(t, conn, "core/1795772278_a_saved_view_name_is_unique_for_its_owner_and_resource.up.sql")
	ns.Name = "saved_view_names_restore"
	ns.Migrations[0].UpSQL = string(up)
	if _, err := dbmigrate.Up(ctx, conn, ns); err != nil {
		t.Fatal(err)
	}

	var retired bool
	if err := conn.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM saved_view WHERE id = $1`, retiredDuplicate).Scan(&retired); err != nil {
		t.Fatal(err)
	}
	if !retired {
		t.Fatal("removed owner's duplicate stayed live")
	}
	for id, want := range map[string]string{first: "Focus", second: "focus (3)", held: "focus (2)", third: "FOCUS (4)"} {
		var got string
		if err := conn.QueryRow(ctx, `SELECT name FROM saved_view WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("name = %q, want %q", got, want)
		}
	}
}
