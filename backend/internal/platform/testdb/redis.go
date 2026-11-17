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
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// BusRedisDBs is the highest database a test may hand to the api or the worker
// in a bus address, whose parser accepts no higher index. The gate holds the
// parser to it.
const BusRedisDBs = 80

// The lease keys live in db 0 of the test Redis, which no test leases. They
// must be on the server itself: the lanes run packages as separate processes,
// and only the server sees every holder. leasePrefix names the key of one
// database, and its value is the holder's token, so only it can renew or end it.
const leasePrefix = "margince-test:redis-lease:"

// composeFile declares both Redis servers. A lease reads the test server's
// database count from it, so the count is stated in one place.
const composeFile = "docker-compose.dev.yml"

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
	return leaseRedisDB(t, 0)
}

// RedisBusDB is RedisDB for a test that passes its database to the api or the
// worker as a bus address. It leases no index above BusRedisDBs.
func RedisBusDB(t testing.TB) int {
	t.Helper()
	db := leaseRedisDB(t, BusRedisDBs)
	if db > BusRedisDBs {
		t.Fatalf("this test already leased redis db %d, which a bus address cannot name: call RedisBusDB before RedisDB", db)
	}
	return db
}

// leaseRedisDB leases a database no higher than highest, or than the server
// serves when highest is 0.
func leaseRedisDB(t testing.TB, highest int) int {
	t.Helper()
	if db, ok := redisLeases.Load(t); ok {
		if n, isInt := db.(int); isInt {
			return n
		}
	}
	addr := RedisAddr(t)
	served := testRedisDatabases(t, addr)
	if highest == 0 || highest > served-1 {
		highest = served - 1
	}
	token := leaseToken(t)
	ledger := redis.NewClient(&redis.Options{Addr: addr, DB: 0})
	db := takeLease(t, ledger, addr, token, highest)
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

// testRedisDatabases is how many databases the server at addr serves. It
// refuses a server whose count is not the one compose gives redis-test, and
// the gate holds the dev Redis to another count. A lease on the dev Redis
// would flush the databases of running dev stacks.
func testRedisDatabases(t testing.TB, addr string) int {
	t.Helper()
	want, err := ComposeRedisDatabases("redis-test")
	if err != nil {
		t.Fatalf("reading the test Redis from %s: %v", composeFile, err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 0})
	defer closeClient(t, rdb)
	res, err := rdb.ConfigGet(t.Context(), "databases").Result()
	if err != nil {
		t.Fatalf("asking the Redis at %s for its database count (run `make db-up`): %v", addr, err)
	}
	if served := res["databases"]; served != strconv.Itoa(want) {
		t.Fatalf("the Redis at %s serves %q databases and redis-test serves %d, so it is not the test Redis: run `make db-up`, or recreate a redis-test started before the count changed",
			addr, served, want)
	}
	return want
}

// composeRedisDecl matches a Redis service's --databases. It is anchored to the
// service key and its command, so a comment naming a number cannot match.
func composeRedisDecl(service string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(service) +
		`:\n(?:    .*\n)*?    command: \["redis-server", "--databases", "(\d+)"`)
}

// ComposeRedisDatabases is the --databases the compose file gives a Redis
// service. It finds the file in the nearest directory above the test's own.
func ComposeRedisDatabases(service string) (int, error) {
	body, err := readComposeFile()
	if err != nil {
		return 0, err
	}
	m := composeRedisDecl(service).FindSubmatch(body)
	if m == nil {
		return 0, fmt.Errorf("%s declares no --databases for the %s service", composeFile, service)
	}
	return strconv.Atoi(string(m[1]))
}

func readComposeFile() ([]byte, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for {
		body, err := os.ReadFile(filepath.Join(dir, composeFile))
		if err == nil {
			return body, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("no %s above the working directory", composeFile)
		}
		dir = parent
	}
}

// takeLease waits up to leaseWait for a free database. Only a running test
// holds a lease, so a wait ends when one finishes. A server still full past
// that wait has more tests using Redis at once than it has databases.
func takeLease(t testing.TB, ledger *redis.Client, addr, token string, highest int) int {
	t.Helper()
	deadline := time.Now().Add(leaseWait)
	for {
		db, err := acquireLease.Run(t.Context(), ledger, nil,
			token, highest, leaseTTL.Milliseconds(), leasePrefix).Int()
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
				highest, addr, leaseWait, leasePrefix)
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
