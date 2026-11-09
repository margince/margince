// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import "testing"

func TestWorkerPoolHonorsTheTightestResourceLimit(t *testing.T) {
	for _, tc := range []struct {
		name, dsn           string
		jobs, budget, cache int
		want                int32
		wantCache           int
	}{
		{"worker budget", "postgres://localhost/test", 38, 8, 64, 7, 64},
		{"small job fleet", "postgres://localhost/test", 1, 8, 64, 4, 64},
		{"DSN ceiling", "postgres://localhost/test?pool_max_conns=6&statement_cache_capacity=16", 38, 8, 64, 6, 16},
		{"budget below DSN", "postgres://localhost/test?pool_max_conns=32", 38, 8, 64, 7, 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := WorkerPoolConfig(tc.dsn, tc.jobs, tc.budget, tc.cache)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.MaxConns != tc.want || cfg.ConnConfig.StatementCacheCapacity != tc.wantCache {
				t.Fatalf("max=%d cache=%d want=%d/%d", cfg.MaxConns, cfg.ConnConfig.StatementCacheCapacity, tc.want, tc.wantCache)
			}
			if cfg.ConnConfig.DescriptionCacheCapacity > tc.cache {
				t.Fatal("description cache exceeds budget")
			}
			if int32(WorkerSessionCapacity(cfg.MaxConns)*2+WorkerControlConnections) > cfg.MaxConns {
				t.Fatal("pinned locks consume the work budget")
			}
		})
	}
}

func TestWorkerPoolRejectsUnusableBudgetsAndCacheModes(t *testing.T) {
	for _, tc := range []struct {
		dsn                 string
		jobs, budget, cache int
	}{
		{"postgres://localhost/test", 0, 8, 64},
		{"postgres://localhost/test", 38, 4, 64},
		{"postgres://localhost/test?pool_max_conns=2", 38, 8, 64},
		{"postgres://localhost/test", 38, 8, -1},
		{"postgres://localhost/test", 38, 8, 0},
	} {
		if _, err := WorkerPoolConfig(tc.dsn, tc.jobs, tc.budget, tc.cache); err == nil {
			t.Fatalf("accepted unusable worker config: %+v", tc)
		}
	}
	cfg, err := WorkerPoolConfig("postgres://localhost/test?default_query_exec_mode=exec", 38, 8, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.StatementCacheCapacity != 0 {
		t.Fatal("cache-free execution ignored")
	}
}
