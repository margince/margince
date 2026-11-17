// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb_test

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/platform/events"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

// How many logical databases each Redis serves is stated in places that cannot
// check each other. They are this package's RedisDBs, --databases for the two
// compose services, DEV_REDIS_DB_MAX in scripts/dev.sh and the bus parser.
// Drift fails a suite with `ERR DB index is out of range`, naming Redis rather
// than the mismatch, or makes a lease unable to tell the two servers apart.
//
// Paths are relative to this package, which is three levels under backend/.
const (
	composePath   = "../../../../docker-compose.dev.yml"
	devScriptPath = "../../../../scripts/dev.sh"
)

// The declarations this reads. Anchored to the assignment rather than any
// mention, so a comment naming the number cannot satisfy the gate.
var (
	devComposeDecl  = regexp.MustCompile(`(?m)^  redis:\n(?:    .*\n)*?    command: \["redis-server", "--databases", "(\d+)"`)
	testComposeDecl = regexp.MustCompile(`(?m)^  redis-test:\n(?:    .*\n)*?    command: \["redis-server", "--databases", "(\d+)"`)
	devMaxDecl      = regexp.MustCompile(`(?m)^DEV_REDIS_DB_MAX=(\d+)$`)
)

func TestRedisDBCountAgreesWithTheComposeFileAndTheServer(t *testing.T) {
	t.Parallel()
	t.Run("the test server serves the leased block and the lease db", func(t *testing.T) {
		if got := declaredInt(t, composePath, testComposeDecl); got != testdb.RedisDBs+1 {
			t.Errorf("redis-test serves --databases %d in %s but testdb.RedisDBs=%d: it must serve db 0 for the leases and 1..%d to lease",
				got, composePath, testdb.RedisDBs, testdb.RedisDBs)
		}
	})

	// A lease refuses a server whose count is not the test server's, which is
	// how it never flushes a dev stack's database.
	t.Run("the dev server serves another count", func(t *testing.T) {
		if got := declaredInt(t, composePath, devComposeDecl); got == testdb.RedisDBs+1 {
			t.Errorf("redis serves --databases %d in %s, the same as redis-test: a lease pointed at the dev Redis would flush dev stacks' databases",
				got, composePath)
		}
	})

	t.Run("the dev server provisions every slugged stack", func(t *testing.T) {
		devMax := declaredInt(t, devScriptPath, devMaxDecl)
		if got := declaredInt(t, composePath, devComposeDecl); got != devMax+1 {
			t.Errorf("--databases %d in %s but DEV_REDIS_DB_MAX=%d: Redis serves 0..databases-1, so it must be asked for %d for the highest slugged stack's db to exist",
				got, composePath, devMax, devMax+1)
		}
	})

	// A test hands its leased db to the worker and the api as an address.
	t.Run("the bus address parser accepts every leased db", func(t *testing.T) {
		if _, err := events.ClientOptions(fmt.Sprintf("localhost:1/%d", testdb.RedisDBs), ""); err != nil {
			t.Errorf("the highest leased db %d is refused as a bus address: %v", testdb.RedisDBs, err)
		}
	})

	// The file is what the container was asked for; this is what it actually
	// serves. They differ whenever a container outlives a change to the file.
	t.Run("the running test server serves that many", func(t *testing.T) {
		addr := testdb.RedisAddr(t)
		rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 0})
		t.Cleanup(func() {
			if err := rdb.Close(); err != nil {
				t.Errorf("closing redis: %v", err)
			}
		})
		res, err := rdb.ConfigGet(t.Context(), "databases").Result()
		if err != nil {
			t.Fatalf("reading the server's database count: %v", err)
		}
		if served := res["databases"]; served != strconv.Itoa(testdb.RedisDBs+1) {
			t.Errorf("the server at %s serves %q databases but the test Redis serves %d: recreate the container (`make db-down && make db-up`); a container started before a change keeps the old count",
				addr, served, testdb.RedisDBs+1)
		}
	})
}

func TestALeaseIsTheTestsOwnUntilItEnds(t *testing.T) {
	t.Parallel()
	addr := testdb.RedisAddr(t)
	var first, second int
	key := "lease-probe:" + t.Name()
	t.Run("holder", func(t *testing.T) {
		first = testdb.RedisDB(t)
		if again := testdb.RedisDB(t); again != first {
			t.Errorf("a second call in one test leased db %d, want the test's db %d", again, first)
		}
		t.Run("a test running beside it", func(t *testing.T) {
			second = testdb.RedisDB(t)
		})
		writeProbe(t, addr, first, key)
	})
	if first == second {
		t.Errorf("two tests running at once both leased db %d", first)
	}
	if n := countProbe(t, addr, first, key); n != 0 {
		t.Errorf("db %d still holds the key its holder wrote after the lease ended", first)
	}
}

func writeProbe(t *testing.T, addr string, db int, key string) {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: db})
	defer closeProbe(t, rdb)
	if err := rdb.Set(t.Context(), key, "1", 0).Err(); err != nil {
		t.Fatalf("writing to leased db %d: %v", db, err)
	}
}

func countProbe(t *testing.T, addr string, db int, key string) int64 {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: db})
	defer closeProbe(t, rdb)
	n, err := rdb.Exists(t.Context(), key).Result()
	if err != nil {
		t.Fatalf("reading db %d: %v", db, err)
	}
	return n
}

func closeProbe(t *testing.T, rdb *redis.Client) {
	if err := rdb.Close(); err != nil {
		t.Errorf("closing redis: %v", err)
	}
}

// declaredInt reads the single number want matches in the file at path, failing
// when the file does not exist or the declaration has moved — a gate that
// silently matches nothing proves nothing.
func declaredInt(t *testing.T, path string, want *regexp.Regexp) int {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	m := want.FindSubmatch(body)
	if m == nil {
		t.Fatalf("%s no longer declares %s — this gate reads that declaration, so it must be updated with the file", path, want)
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("%s declares %q, which is not a number", path, m[1])
	}
	return n
}
