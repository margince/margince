// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package redistest is the shared flushed-Redis fixture for the integration
// lanes. It lives in the platform tier — the only tier the arch rules let
// import a raw Redis client — so a suite in the module or composition tier
// gets a real client WITHOUT importing go-redis itself.
package redistest

import (
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// Client returns a flushed Redis client on the isolated integration db the
// parallel runner assigned this package — see testdb.RedisDB. It fails loudly
// (never skips) when Redis is not provisioned — the same posture the DB fixtures
// take.
func Client(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("MARGINCE_TEST_REDIS")
	if addr == "" {
		t.Fatal("MARGINCE_TEST_REDIS not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: testdb.RedisDB(t)})
	// Register cleanup immediately, before the fatal Ping/FlushDB paths, so
	// a setup failure still closes the client instead of leaking it.
	t.Cleanup(func() {
		if err := rdb.Close(); err != nil {
			t.Errorf("closing redis: %v", err)
		}
	})
	if err := rdb.Ping(t.Context()).Err(); err != nil {
		t.Fatalf("redis at %s unreachable — run `make db-up`: %v", addr, err)
	}
	if err := rdb.FlushDB(t.Context()).Err(); err != nil {
		t.Fatalf("flushing test redis db: %v", err)
	}
	return rdb
}
