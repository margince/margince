// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ratelimit

// Real-Redis lane for the whole reason this store exists: a ceiling is ONE
// ceiling across replicas, not one per replica.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// sharedTestRedis is a per-test Redis db (never db 0, which a running
// `make dev` owns), flushed clean so a count assertion never inherits another
// test's keys.
func sharedTestRedis(t *testing.T) (context.Context, *redis.Client) {
	t.Helper()
	addr := os.Getenv("MARGINCE_TEST_REDIS")
	if addr == "" {
		t.Fatal("MARGINCE_TEST_REDIS not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := t.Context()
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: testdb.RedisDB(t)})
	t.Cleanup(func() {
		if err := rdb.Close(); err != nil {
			t.Errorf("closing the test redis client: %v", err)
		}
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis at %s unreachable — run `make db-up`: %v", addr, err)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushing test redis db: %v", err)
	}
	return ctx, rdb
}

// replicas builds what two processes holding the same ceiling look like: two
// registries, two limiters, no shared memory between them — only the store.
// Separate clients as well as separate registries, because a single client
// shared by both would prove rather less than the deployment does.
func replicas(t *testing.T, name string, kind Kind, limit int, span time.Duration) (*Limiter, *Limiter) {
	t.Helper()
	_, rdb := sharedTestRedis(t)
	addr := os.Getenv("MARGINCE_TEST_REDIS")
	second := redis.NewClient(&redis.Options{Addr: addr, DB: testdb.RedisDB(t)})
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("closing the second replica's redis client: %v", err)
		}
	})
	return Shared(rdb).New(name, kind, limit, span), Shared(second).New(name, kind, limit, span)
}

// The assertion this file exists for. Two limiters that share one Redis spend
// ONE budget between them: four attempts against a limit of three are refused
// on the fourth wherever it lands, where two in-process limiters would have
// admitted six. This is also what fails if somebody later puts a local map
// back in front of the shared store as a cache — the two replicas would each
// admit three again, and every single-process test would still pass.
func TestTwoReplicasSharingOneRedisEnforceOneCeiling(t *testing.T) {
	a, b := replicas(t, "test/one-ceiling", FailClosed, 3, time.Minute)

	for i, l := range []*Limiter{a, b, a} {
		if !l.Allow("alice") {
			t.Fatalf("attempt %d of 3 was refused; the shared budget is being spent too fast", i+1)
		}
	}
	if b.Allow("alice") {
		t.Error("the fourth attempt was admitted by the other replica; each is counting its own ceiling")
	}
	if !a.Allow("bob") {
		t.Error("another key must not share alice's window")
	}
}

// Blocked peeks and Record spends, and that split has to survive the move to a
// shared store or comms.MailboxRatePolicy silently stops pacing: it asks
// Blocked before every send, so a Blocked that spent a slot would exhaust the
// mailbox's window by asking about it.
func TestBlockedPeeksAcrossReplicasWithoutSpendingASlot(t *testing.T) {
	a, b := replicas(t, "test/peek-across", FailOpen, 2, time.Minute)

	for i := 0; i < 5; i++ {
		if b.Blocked("mailbox") {
			t.Fatalf("probe %d reported the mailbox spent before anything was recorded", i+1)
		}
	}
	a.Record("mailbox")
	if b.Blocked("mailbox") {
		t.Fatal("one of two slots is spent; the other replica must not report the mailbox blocked")
	}
	a.Record("mailbox")
	if !b.Blocked("mailbox") {
		t.Error("both slots are spent on one replica; the other must see the mailbox blocked")
	}
}

// The window is the store's, measured from the event that opened it, so a
// counted key must carry an expiry. Without one a key that hit its limit would
// hold that count for the life of the server: a caller limited once would be
// limited forever, on every replica, with no way back but a manual delete.
func TestASharedWindowExpires(t *testing.T) {
	ctx, rdb := sharedTestRedis(t)
	l := Shared(rdb).New("test/expiry", FailClosed, 1, 90*time.Second)

	if !l.Allow("k") {
		t.Fatal("the first attempt must be admitted")
	}
	ttl, err := rdb.PTTL(ctx, "ratelimit:test/expiry:k").Result()
	if err != nil {
		t.Fatalf("reading the counter's ttl: %v", err)
	}
	if ttl <= 0 || ttl > 90*time.Second {
		t.Errorf("the counter's ttl is %s, want a remainder of the 90s window", ttl)
	}
}

// Reset is the non-production data reset's path: the operator who just wiped
// the installation must not be the one the lockout is still holding out, and
// with a shared store the counters it has to reach are on a server the wipe
// does not otherwise touch. Scoped to the limiter doing the resetting, because
// clearing every ceiling in the store would clear ones no operator asked about.
func TestResetClearsTheSharedBucketsItOwnsAndNoOthers(t *testing.T) {
	ctx, rdb := sharedTestRedis(t)
	reg := Shared(rdb)
	spent := reg.New("test/reset-mine", FailClosed, 1, time.Minute)
	neighbour := reg.New("test/reset-theirs", FailClosed, 1, time.Minute)

	for _, l := range []*Limiter{spent, neighbour} {
		if !l.Allow("k") {
			t.Fatalf("the first attempt on %s must be admitted", l.name)
		}
		if l.Allow("k") {
			t.Fatalf("the second attempt on %s must be refused; the bucket is spent", l.name)
		}
	}

	spent.Reset()

	if !spent.Allow("k") {
		t.Error("Allow refused after Reset; the shared bucket was not cleared")
	}
	if neighbour.Allow("k") {
		t.Error("Reset cleared a ceiling nobody asked it to clear")
	}
	if n, err := rdb.Exists(ctx, "ratelimit:test/reset-theirs:k").Result(); err != nil || n != 1 {
		t.Errorf("the neighbour's counter is gone from the store (exists=%d, err=%v)", n, err)
	}
}
