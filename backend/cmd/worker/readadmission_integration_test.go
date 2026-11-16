// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

//testdb:serial binds the process-wide configuration (config.Bind)
func TestBootAppliesTheConfiguredReadAllowance(t *testing.T) {
	cfg, _, err := loadWorkerConfig(nil, []string{"MARGINCE_ENV=dev", "MARGINCE_DSN=" + testdb.For(t).AppDSN, "MARGINCE_DATABASE_READ_CONCURRENCY=1"})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := openWorkerPool(t.Context(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if got := database.ReadAdmissionStats(pool).Capacity; got != 1 {
		t.Fatalf("boot read allowance = %d, want configured 1", got)
	}
}
