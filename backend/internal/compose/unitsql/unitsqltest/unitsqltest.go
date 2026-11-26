// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package unitsqltest makes a unit-owned table for a test, the way a unit's
// migration leaves one. It sits in schema ext and the migrate role creates it,
// so the core migration's default privileges give ext_runtime its rights.
package unitsqltest

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// ScratchTable creates table (id uuid PRIMARY KEY, body text) and drops it when
// the test ends. table names its schema, as in "ext.ext_alpha_probe".
func ScratchTable(t *testing.T, table string) {
	t.Helper()
	ctx := context.Background()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS `+table); err != nil {
			t.Errorf("dropping the unit's own table: %v", err)
		}
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS ` + table,
		`CREATE TABLE ` + table + ` (id uuid PRIMARY KEY, body text)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ` + table + ` TO margince_app`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing the unit's own table: %v", err)
		}
	}
}
