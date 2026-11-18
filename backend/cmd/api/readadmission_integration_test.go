// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

import (
	"os"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
)

func TestBootAppliesTheConfiguredReadAllowance(t *testing.T) {
	dsn := os.Getenv("MARGINCE_TEST_APP_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_APP_DSN is required; run through make test-it")
	}
	cfg, _, err := loadAPIConfig(nil, []string{"MARGINCE_ENV=dev", "MARGINCE_DSN=" + dsn, "MARGINCE_DATABASE_READ_CONCURRENCY=1", "MARGINCE_DATABASE_CONNECTION_LIFETIME=30m", "MARGINCE_DATABASE_CONNECTION_IDLE_TIME=2m"})
	if err != nil {
		t.Fatal(err)
	}
	pool, _, err := openInstallation(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if got := pool.Config(); got.MaxConnLifetime != 30*time.Minute || got.MaxConnIdleTime != 2*time.Minute || got.MaxConnLifetimeJitter != 6*time.Minute {
		t.Fatalf("boot connection ages: lifetime=%s idle=%s jitter=%s", got.MaxConnLifetime, got.MaxConnIdleTime, got.MaxConnLifetimeJitter)
	}

	if got := database.ReadAdmissionStats(pool).Capacity; got != 1 {
		t.Fatalf("boot read allowance = %d, want configured 1", got)
	}
}
