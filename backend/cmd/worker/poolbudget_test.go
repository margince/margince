// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import "testing"

func TestWorkerDatabaseBudgetIsParsedAndValidatedAtBoot(t *testing.T) {
	cfg, _, err := loadWorkerConfig(nil, []string{workerTestDSN})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.databaseConnections != 12 || cfg.databaseCache != 256 {
		t.Fatalf("defaults=%d/%d", cfg.databaseConnections, cfg.databaseCache)
	}
	cfg, _, err = loadWorkerConfig([]string{"--worker-max-connections=12", "--worker-statement-cache-capacity=32"}, []string{workerTestDSN})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.databaseConnections != 12 || cfg.databaseCache != 32 {
		t.Fatal("explicit worker budget ignored")
	}
	for _, args := range [][]string{{"--worker-max-connections=4"}, {"--worker-statement-cache-capacity=-1"}, {"--worker-statement-cache-capacity=0"}} {
		if _, _, err := loadWorkerConfig(args, []string{workerTestDSN}); err == nil {
			t.Fatalf("accepted unusable budget: %v", args)
		}
	}
	for _, dsn := range []string{"MARGINCE_DSN=postgres://localhost/test?pool_max_conns=3", "MARGINCE_DSN=postgres://localhost/test?statement_cache_capacity=0"} {
		if _, _, err := loadWorkerConfig(nil, []string{dsn}); err == nil {
			t.Fatalf("accepted unusable DSN budget: %s", dsn)
		}
	}
}
