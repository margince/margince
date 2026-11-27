// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package unitsqltest makes a unit-owned table for a test, the way a unit's
// migration leaves one. It sits in schema ext and the migrate role creates it,
// so the core migration's default privileges give ext_runtime its rights.
package unitsqltest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// ScratchTable creates table (id uuid PRIMARY KEY, body text) in the test's
// own database. table names its schema, as in "ext.ext_alpha_probe".
func ScratchTable(t *testing.T, table string) {
	t.Helper()
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, testdb.For(t).OwnerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	for _, stmt := range []string{
		`CREATE TABLE ` + table + ` (id uuid PRIMARY KEY, body text)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ` + table + ` TO margince_app`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing the unit's own table: %v", err)
		}
	}
}
