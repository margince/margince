// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// A host left with two live pages keeps the one in use. That is the one their
// alias names, else the one that was used, else the newest.
func TestOnePersonalPageKeepsThePageInUse(t *testing.T) {
	t.Parallel()
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1795540594")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("rolling the index back: %v", err)
	}

	used := seedAppUserForConnection(t, conn, "used@personal.example")
	aliased := seedAppUserForConnection(t, conn, "aliased@personal.example")
	older := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	seed := func(host, slug string, created time.Time, firstUse *time.Time) {
		if _, err := conn.Exec(ctx, `INSERT INTO booking_page (host_user_id, slug, created_at, first_used_at) VALUES ($1, $2, $3, $4)`,
			host, slug, created, firstUse); err != nil {
			t.Fatalf("seeding page %s: %v", slug, err)
		}
	}
	seed(used, "used-older-slug", older, &older)
	seed(used, "used-newer-slug", older.Add(time.Hour), nil)
	seed(aliased, "aliased-older-slug", older, nil)
	seed(aliased, "aliased-newer-slug", older.Add(time.Hour), &older)
	if _, err := conn.Exec(ctx, `INSERT INTO booking_alias (alias, host_user_id, booking_page_id)
		SELECT 'aliased-host', host_user_id, id FROM booking_page WHERE slug = 'aliased-older-slug'`); err != nil {
		t.Fatalf("seeding the alias: %v", err)
	}

	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	for host, want := range map[string]string{used: "used-older-slug", aliased: "aliased-older-slug"} {
		var live []string
		rows, err := conn.Query(ctx, `SELECT slug FROM booking_page WHERE host_user_id = $1 AND revoked_at IS NULL`, host)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var slug string
			if err := rows.Scan(&slug); err != nil {
				t.Fatal(err)
			}
			live = append(live, slug)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if len(live) != 1 || live[0] != want {
			t.Errorf("host %s keeps %v live, want only %s", host, live, want)
		}
	}
}
