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

// holdsTaxRateGrant reports whether the system role names the tax_rate object.
func holdsTaxRateGrant(t *testing.T, conn *pgx.Conn, key string) bool {
	t.Helper()
	var holds bool
	if err := conn.QueryRow(context.Background(),
		`SELECT (permissions -> 'objects') ? 'tax_rate' FROM role WHERE key = $1`, key).Scan(&holds); err != nil {
		t.Fatalf("reading role %s: %v", key, err)
	}
	return holds
}

// The tax rate up grants admin the sheet. Its down takes the grant away again,
// so the next up grants it afresh instead of finding a stale key and skipping.
func TestTheTaxRateDownTakesBackTheGrantItsUpGave(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()
	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1793734536")); err != nil {
		t.Fatalf("up to the migration before the tax rate one: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO role (key, name, is_system, permissions)
		VALUES ('admin', 'Admin', true, '{"objects":{}}'::jsonb)
		ON CONFLICT (key) DO UPDATE SET permissions = '{"objects":{}}'::jsonb`); err != nil {
		t.Fatalf("seeding the admin role: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1793741150")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if !holdsTaxRateGrant(t, conn, "admin") {
		t.Fatal("the up gave admin no tax_rate grant")
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 2); err != nil {
		t.Fatalf("down: %v", err)
	}
	if holdsTaxRateGrant(t, conn, "admin") {
		t.Error("the rollback left admin's tax_rate grant behind")
	}
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up again after the rollback: %v", err)
	}
	var grant string
	if err := conn.QueryRow(ctx, `SELECT permissions #>> '{objects,tax_rate}' FROM role WHERE key = 'admin'`).Scan(&grant); err != nil {
		t.Fatal(err)
	}
	if grant != `{"read": true, "create": true, "delete": false, "update": false}` {
		t.Errorf("after the round trip admin holds %s, want create and read", grant)
	}
}
