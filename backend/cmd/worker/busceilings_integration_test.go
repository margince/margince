// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

// The worker's event lanes park bus connections in blocking reads. With every
// bus connection held that way, a shared ceiling must still answer: it rides
// its own client, not the bus pool.
func TestCeilingsStillAnswerWhileEveryBusConnectionIsBlocked(t *testing.T) {
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

	// Hold every bus connection in a blocking read, as the lanes do.
	held := bus.Options().PoolSize
	blockCtx, release := context.WithCancel(t.Context())
	var readers sync.WaitGroup
	for range held {
		readers.Go(func() {
			//craft:ignore swallowed-errors the read only exists to hold a connection; it ends by cancellation
			_ = bus.XRead(blockCtx, &redis.XReadArgs{Streams: []string{"test:ceilings:idle", "$"}, Block: 3 * time.Second}).Err()
		})
	}
	t.Cleanup(func() {
		release()
		readers.Wait()
		closeBus(slog.New(slog.NewTextHandler(io.Discard, nil)), bus, coordination)
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		stats := bus.PoolStats()
		if int(stats.TotalConns) >= held && stats.IdleConns == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bus pool never filled: %+v of %d", stats, held)
		}
		time.Sleep(20 * time.Millisecond)
	}

	pacer := ratelimit.NewPacer()
	if _, _, err := pacer.Reserve(t.Context(), ratelimit.Budget{Key: "test/ceilings/" + t.Name(), PerMinute: 600, Cost: 1}); err != nil {
		t.Fatalf("a ceiling failed while the bus pool was busy: %v", err)
	}
}
