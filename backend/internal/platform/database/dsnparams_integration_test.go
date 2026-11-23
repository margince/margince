// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
)

// A DSN that asks for JIT does not get it: row-scope predicates price a search
// statement past jit_above_cost, and compiling it took seconds.
func TestAPoolFromADSNThatAsksForJITRunsWithJITOff(t *testing.T) {
	ctx := context.Background()
	pool, err := database.NewPool(ctx, database.WithDSNParam(os.Getenv("MARGINCE_TEST_APP_DSN"), "jit", "on"))
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
