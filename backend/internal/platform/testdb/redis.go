// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisDBs is how many logical databases the test Redis leases out, 1..RedisDBs.
// Db 0 holds the lease keys, so the server serves one more than this. The dev
// Redis serves another count, which is how a lease tells the two servers apart.
const RedisDBs = 64

// leasePrefix names the db 0 key that holds a lease on the database its suffix
// names. The value is the holder's token, so only the holder can renew or end it.
const leasePrefix = "margince-test:redis-lease:"

// A lease outlives its holder by at most leaseTTL, which is how a process that
// was killed gives its databases back. The holder renews well inside it.
const (
	leaseTTL   = 2 * time.Minute
	leaseRenew = 20 * time.Second
	leaseWait  = 2 * time.Minute
	leasePoll  = 250 * time.Millisecond
)

// acquireLease takes the first free database in one round trip, so two
// processes that race for one database cannot both win it.
var acquireLease = redis.NewScript(`
for db = 1, tonumber(ARGV[2]) do
  if redis.call('SET', ARGV[4] .. db, ARGV[1], 'NX', 'PX', ARGV[3]) then
    return db
  end
end
return 0`)

// renewLease and endLease act only on a lease the token still holds. A lease
// that expired and went to another test is that test's, not ours to touch.
var (
	renewLease = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
return 0`)
	endLease = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0`)
)

// redisLeases holds the database each running test leased, so a test that
// opens a second client reaches the data its first one wrote.
var redisLeases sync.Map

// RedisAddr is the address of the test Redis, which `make db-up` starts as the
// redis-test service. It fails the test when unset: an integration test never skips.
func RedisAddr(t testing.TB) string {
	t.Helper()
	addr := os.Getenv("MARGINCE_TEST_REDIS")
	if addr == "" {
		t.Fatal("MARGINCE_TEST_REDIS not set: run `make db-up` (integration tests fail loudly, they never skip)")
	}
	return addr
}

// RedisDB is the one spelling of which Redis logical database a test may touch.
// The first call in a test leases a free, flushed database on the test Redis
// for the life of that test. The lease is held across every process that uses
// the server, so tests in parallel and packages in parallel never share one.
// At the end of the test the database is flushed and the lease released.
func RedisDB(t testing.TB) int {
	t.Helper()
	if db, ok := redisLeases.Load(t); ok {
		if n, isInt := db.(int); isInt {
			return n
		}
	}
	addr := RedisAddr(t)
	token := leaseToken(t)
	ledger := redis.NewClient(&redis.Options{Addr: addr, DB: 0})
	db := takeLease(t, ledger, addr, token)
	stop := keepLease(t, ledger, db, token)
	redisLeases.Store(t, db)
	t.Cleanup(func() {
		redisLeases.Delete(t)
		stop()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := flushRedisDB(ctx, addr, db); err != nil {
			t.Errorf("flushing test redis db %d before releasing it: %v", db, err)
		}
		if err := endLease.Run(ctx, ledger, []string{leaseKey(db)}, token).Err(); err != nil {
			t.Errorf("releasing the lease on test redis db %d: %v", db, err)
		}
		closeClient(t, ledger)
	})
	// A holder that was killed left its keys behind when its lease expired.
	if err := flushRedisDB(t.Context(), addr, db); err != nil {
		t.Fatalf("flushing the leased test redis db %d: %v", db, err)
	}
	return db
}

// takeLease waits up to leaseWait for a free database. Only a running test
// holds a lease, so a wait ends when one finishes. A server still full past
// that wait has more tests using Redis at once than it has databases.
func takeLease(t testing.TB, ledger *redis.Client, addr, token string) int {
	t.Helper()
	if err := isTestServer(t.Context(), ledger); err != nil {
		closeClient(t, ledger)
		t.Fatalf("the Redis at %s is not the test Redis (run `make db-up`, which starts redis-test): %v", addr, err)
	}
	deadline := time.Now().Add(leaseWait)
	for {
		db, err := acquireLease.Run(t.Context(), ledger, nil,
			token, RedisDBs, leaseTTL.Milliseconds(), leasePrefix).Int()
		if err != nil {
			closeClient(t, ledger)
			t.Fatalf("leasing a database on the test redis at %s (run `make db-up`): %v", addr, err)
		}
		if db > 0 {
			return db
		}
		if time.Now().After(deadline) {
			closeClient(t, ledger)
			t.Fatalf("every one of the %d databases on the test redis at %s stayed leased for %s: more tests use Redis at once than it serves; list the holders with `redis-cli -p <port> --scan --pattern '%s*'`",
				RedisDBs, addr, leaseWait, leasePrefix)
		}
		time.Sleep(leasePoll)
	}
}

// keepLease renews the lease until the returned stop is called. Stop waits for
// the renewer to end, so no renewal runs after the lease is released.
func keepLease(t testing.TB, ledger *redis.Client, db int, token string) (stop func()) {
	done := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		tick := time.NewTicker(leaseRenew)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				held, err := renew(ledger, db, token)
				if err != nil {
					t.Errorf("renewing the lease on test redis db %d: %v", db, err)
				} else if held == 0 {
					t.Errorf("the lease on test redis db %d expired while the test held it, so another test may share the database", db)
				}
			}
		}
	}()
	return func() {
		close(done)
		<-ended
	}
}

// isTestServer refuses a server that does not serve the leased block and db 0
// alone. A lease on the dev Redis would flush the databases of running dev
// stacks.
func isTestServer(ctx context.Context, rdb *redis.Client) error {
	res, err := rdb.ConfigGet(ctx, "databases").Result()
	if err != nil {
		return err
	}
	if served := res["databases"]; served != strconv.Itoa(RedisDBs+1) {
		return fmt.Errorf("it serves %q databases, and the test Redis serves %d", served, RedisDBs+1)
	}
	return nil
}

func flushRedisDB(ctx context.Context, addr string, db int) error {
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: db})
	return errors.Join(rdb.FlushDB(ctx).Err(), rdb.Close())
}

func closeClient(t testing.TB, rdb *redis.Client) {
	if err := rdb.Close(); err != nil {
		t.Errorf("closing a test redis client: %v", err)
	}
}

func renew(ledger *redis.Client, db int, token string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return renewLease.Run(ctx, ledger, []string{leaseKey(db)}, token, leaseTTL.Milliseconds()).Int()
}

func leaseKey(db int) string { return leasePrefix + strconv.Itoa(db) }

// leaseToken is unique to one lease, so a test can tell its own lease from the
// one another test took after it expired.
func leaseToken(t testing.TB) string {
	t.Helper()
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatalf("drawing a redis lease token: %v", err)
	}
	return hex.EncodeToString(raw[:])
}
