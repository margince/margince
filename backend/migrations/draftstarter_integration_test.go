// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheDraftStarterUpgradePausesOnlyUnownedDrafts(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	owner := ids.NewV7()
	if _, err := conn.Exec(ctx, "INSERT INTO app_user(id,email,display_name) VALUES ($1,'author@example.test','Author')", owner); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO automation(key,name,trigger,action,owner_id,enabled)
 VALUES ('unowned','unowned','{}','{"kind":"draft_email"}',NULL,true),
 ('owned','owned','{}','{"kind":"draft_email"}',$1,true),
 ('system','system','{}','{"kind":"create_task"}',NULL,true)`, owner); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("core/1788610228_an_unowned_draft_starter_waits_for_an_author.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := conn.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := conn.Query(ctx, "SELECT key,enabled,version FROM automation WHERE key IN ('unowned','owned','system') ORDER BY key")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for rows.Next() {
		var key string
		var enabled bool
		var version int
		if err := rows.Scan(&key, &enabled, &version); err != nil {
			t.Fatal(err)
		}
		wantEnabled, wantVersion := key != "unowned", 1
		if !wantEnabled {
			wantVersion = 2
		}
		if enabled != wantEnabled || version != wantVersion {
			t.Fatalf("%s is enabled=%v version=%d", key, enabled, version)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if count != 3 {
		t.Fatalf("read %d automations, want 3", count)
	}
	var audited int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE entity_type='automation' AND entity_id IN (SELECT id FROM automation WHERE key='unowned')").Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Fatalf("pause was audited %d times, want once", audited)
	}
}
