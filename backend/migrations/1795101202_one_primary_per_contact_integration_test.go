// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// A contact that held a primary address of each type keeps the one the app
// already used. The per-contact index then refuses a second primary.
func TestTheOnePrimaryMigrationKeepsTheAddressTheAppUsed(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1795101202")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("restoring the per-type index: %v", err)
	}

	contact := scalar(t, conn, `INSERT INTO contact (full_name, source, captured_by)
		VALUES ('Ada', 'manual', 'human:x') RETURNING id`)
	for _, row := range []struct {
		email, kind string
		position    int
	}{{"work@one.example", "work", 1}, {"home@one.example", "personal", 0}, {"other@one.example", "other", 2}} {
		if _, err := conn.Exec(ctx, `INSERT INTO contact_email (contact_id, email, email_type, is_primary, position, source, captured_by)
			VALUES ($1, $2, $3, true, $4, 'manual', 'human:x')`, contact, row.email, row.kind, row.position); err != nil {
			t.Fatalf("seeding a primary %s address: %v", row.kind, err)
		}
	}
	for _, row := range []struct {
		phone, kind string
		position    int
	}{{"+493011111111", "work", 0}, {"+493022222222", "mobile", 1}} {
		if _, err := conn.Exec(ctx, `INSERT INTO contact_phone (contact_id, phone, phone_type, is_primary, position, source, captured_by)
			VALUES ($1, $2, $3, true, $4, 'manual', 'human:x')`, contact, row.phone, row.kind, row.position); err != nil {
			t.Fatalf("seeding a primary %s number: %v", row.kind, err)
		}
	}

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	if got := scalar(t, conn, `SELECT string_agg(email, ',') FROM contact_email WHERE contact_id = $1 AND is_primary`, contact); got != "home@one.example" {
		t.Errorf("primary addresses = %q, want only the one at the lowest position", got)
	}
	if got := scalar(t, conn, `SELECT count(*)::text FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL`, contact); got != "3" {
		t.Errorf("live addresses = %s, want all three kept", got)
	}
	if got := scalar(t, conn, `SELECT string_agg(phone, ',') FROM contact_phone WHERE contact_id = $1 AND is_primary`, contact); got != "+493011111111" {
		t.Errorf("primary numbers = %q, want only the one at the lowest position", got)
	}
	if _, err := conn.Exec(ctx, `UPDATE contact_email SET is_primary = true WHERE contact_id = $1 AND email = 'work@one.example'`, contact); err == nil {
		t.Error("a second primary address was accepted, want the per-contact index to refuse it")
	}
}
