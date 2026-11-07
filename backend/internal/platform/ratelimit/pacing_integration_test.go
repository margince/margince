// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ratelimit

import (
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func TestWeightedPacingIsAtomicAcrossRedisRegistries(t *testing.T) {
	ctx, rdb := sharedTestRedis(t)
	first, second := Shared(rdb).NewPacer(func() time.Time { return clocktest.Now(t) }), Shared(rdb).NewPacer(func() time.Time { return clocktest.Now(t) })
	budgets := []Budget{{Key: "shared-project", PerMinute: 100, Cost: 100}, {Key: "shared-mailbox", PerMinute: 100, Cost: 100}}
	results := make(chan time.Duration, 20)
	var group sync.WaitGroup
	for i := range 20 {
		meter := first
		if i%2 == 0 {
			meter = second
		}
		group.Go(func() {
			wait, _, err := meter.Reserve(ctx, budgets...)
			if err != nil {
				t.Error(err)
			}
			results <- wait
		})
	}
	group.Wait()
	close(results)
	admitted := 0
	for wait := range results {
		if wait == 0 {
			admitted++
		} else if wait <= 50*time.Second || wait > time.Minute {
			t.Errorf("unexpected shared wait %v", wait)
		}
	}
	if admitted != 1 {
		t.Fatalf("%d simultaneous calls admitted across replicas", admitted)
	}
	for _, budget := range budgets {
		key := "ratelimit:pace:{provider}:" + budget.Key
		ttl, err := rdb.PTTL(ctx, key).Result()
		if err != nil || ttl <= 0 || ttl > time.Minute {
			t.Fatalf("unbounded pacing state %v %v", ttl, err)
		}
	}
}

func TestRedisPauseExtendsOneSharedDeadlineWithoutShorteningIt(t *testing.T) {
	ctx, rdb := sharedTestRedis(t)
	first, second := Shared(rdb).NewPacer(func() time.Time { return clocktest.Now(t) }), Shared(rdb).NewPacer(func() time.Time { return clocktest.Now(t) })
	if err := first.Pause(ctx, "mailbox", 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := second.Pause(ctx, "mailbox", time.Second); err != nil {
		t.Fatal(err)
	}
	wait, key, err := second.Reserve(ctx, Budget{Key: "mailbox", PerMinute: 6000, Cost: 60})
	if err != nil || wait < time.Minute || wait > 2*time.Minute || key != pacedKey("mailbox") {
		t.Fatalf("pause not shared: %v %q %v", wait, key, err)
	}
	if delay, _, err := first.Reserve(ctx, Budget{Key: "other-mailbox", PerMinute: 6000, Cost: 60}); err != nil || delay != 0 {
		t.Fatalf("pause leaked scope: %v %v", delay, err)
	}
	ttl, err := rdb.PTTL(ctx, pacedKey("mailbox")).Result()
	if err != nil || ttl < time.Minute || ttl > 2*time.Minute {
		t.Fatalf("pause lost TTL: %v %v", ttl, err)
	}
}

func TestClosedRedisRefusesPacingAndPause(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "unused"})
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	meter := Shared(client).NewPacer(func() time.Time { return clocktest.Now(t) })
	if _, _, err := meter.Reserve(t.Context(), Budget{Key: "mail", PerMinute: 6000, Cost: 60}); err == nil {
		t.Fatal("closed Redis admitted a quota reservation")
	}
	if err := meter.Pause(t.Context(), "mail", time.Minute); err == nil {
		t.Fatal("closed Redis discarded a provider pause")
	}
}
