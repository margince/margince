// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"errors"
	"os"
	"testing"
)

// TestPoolRefusesBeforeTheSchemaIsMigrated drives the one branch that keeps a
// pooled connection from predating the migration's DROP SCHEMA.
//
// It is an internal test because sharedPool is internal. It takes the
// unmigrated state as an argument, so the test never writes the flag other
// tests read.
//
// Without this, deleting the check leaves every suite green while a session from
// a too-early pool blocks the migration's DROP SCHEMA — see ErrSchemaNotReady for
// why that costs the package rather than the one test.
func TestPoolRefusesBeforeTheSchemaIsMigrated(t *testing.T) {
	t.Parallel()
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if appDSN == "" {
		t.Fatal("MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	pool, err := sharedPool(context.Background(), appDSN, false)
	if pool != nil {
		t.Error("Pool handed out a pool for an unmigrated database")
	}
	if !errors.Is(err, ErrSchemaNotReady) {
		t.Fatalf("Pool on an unmigrated database returned %v, want ErrSchemaNotReady", err)
	}
}
