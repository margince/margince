// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

// A DSN that asks for JIT does not get it: row-scope predicates price a search
// statement past jit_above_cost, and compiling it took seconds.
func TestAPoolFromADSNThatAsksForJITRunsWithJITOff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool, err := testdb.OwnPool(ctx, database.WithDSNParam(testdb.For(t).AppDSN, "jit", "on"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var jit string
	if err := pool.QueryRow(ctx, `SHOW jit`).Scan(&jit); err != nil {
		t.Fatal(err)
	}
	if jit != "off" {
		t.Errorf("a pool built from a jit=on DSN reports jit=%s", jit)
	}
}
