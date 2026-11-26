// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
)

func TestTheSizeCounterObservesTheStatementThePoolSends(t *testing.T) {
	ctx, pool := laneEnv(t)
	database.ResetLargestSQLStatementBytes()
	if got := database.LargestSQLStatementBytes(); got != 0 {
		t.Fatalf("statement size before this read = %d, want zero", got)
	}
	query := "SELECT 1 /*" + strings.Repeat("é", 64) + "*/"
	var value int
	if err := pool.QueryRow(ctx, query).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != 1 {
		t.Fatalf("query answered %d", value)
	}
	if got := database.LargestSQLStatementBytes(); got < int64(len(query)) {
		t.Fatalf("pool sent %d SQL bytes, but the counter saw %d", len(query), got)
	}
}
