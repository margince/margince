// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

import (
	"io"
	"log/slog"
	"os"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

// The worker's event lanes keep bus connections checked out in blocking reads.
// With every bus connection taken, a shared ceiling must still answer: it rides
// its own client, not the bus pool.
func TestCeilingsStillAnswerWhileEveryBusConnectionIsTaken(t *testing.T) {
	addr := os.Getenv("MARGINCE_TEST_REDIS")
	if addr == "" {
		t.Skip("MARGINCE_TEST_REDIS not set")
	}
	cfg := workerConfig{redisAddr: addr + "/" + strconv.Itoa(testdb.RedisDB(t))}
	bus, coordination, err := openBus(t.Context(), cfg)
	if err != nil {
		t.Fatalf("open bus: %v", err)
	}
	if bus == coordination {
		t.Fatal("the ceilings share the bus client")
	}

	// Check out every bus connection and keep it, as the lanes' reads do.
	held := make([]*redis.Conn, 0, bus.Options().PoolSize)
	t.Cleanup(func() {
		for _, conn := range held {
			//craft:ignore swallowed-errors returning a held test connection; the client closes next
			_ = conn.Close()
		}
		closeBus(slog.New(slog.NewTextHandler(io.Discard, nil)), bus, coordination)
	})
	for range bus.Options().PoolSize {
		conn := bus.Conn()
		if err := conn.Ping(t.Context()).Err(); err != nil {
			t.Fatalf("hold a bus connection: %v", err)
		}
		held = append(held, conn)
	}
	if stats := bus.PoolStats(); stats.IdleConns != 0 {
		t.Fatalf("the bus pool still has idle connections: %+v", stats)
	}

	pacer := ratelimit.NewPacer()
	if _, _, err := pacer.Reserve(t.Context(), ratelimit.Budget{Key: "test/ceilings/" + t.Name(), PerMinute: 600, Cost: 1}); err != nil {
		t.Fatalf("a ceiling failed while the bus pool was taken: %v", err)
	}
}
