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

// The two Redis servers' database counts are declared in the compose file, and
// a lease reads the test server's from there. These checks hold what a lease
// cannot see. The dev server's count must tell it apart and serve every
// slugged stack, and a leased bus database must parse.
const devScriptPath = "../../../../scripts/dev.sh"

var devMaxDecl = regexp.MustCompile(`(?m)^DEV_REDIS_DB_MAX=(\d+)$`)

func TestRedisDBCountAgreesWithTheComposeFileAndTheServer(t *testing.T) {
	t.Parallel()
	dev, err := testdb.ComposeRedisDatabases("redis")
	if err != nil {
		t.Fatal(err)
	}
	test, err := testdb.ComposeRedisDatabases("redis-test")
	if err != nil {
		t.Fatal(err)
	}

	// A lease refuses a server whose count is not redis-test's, which is how it
	// never flushes a dev stack's database.
	t.Run("the dev server serves another count", func(t *testing.T) {
		if dev == test {
			t.Errorf("redis and redis-test both serve --databases %d: a lease pointed at the dev Redis would flush dev stacks' databases", dev)
		}
	})

	t.Run("the dev server provisions every slugged stack", func(t *testing.T) {
		if devMax := declaredInt(t, devScriptPath, devMaxDecl); dev != devMax+1 {
			t.Errorf("redis serves --databases %d but DEV_REDIS_DB_MAX=%d in %s: Redis serves 0..databases-1, so it must be asked for %d",
				dev, devMax, devScriptPath, devMax+1)
		}
	})

	// A test hands its leased db to the worker and the api as an address.
	t.Run("the bus address parser accepts every bus database", func(t *testing.T) {
		if testdb.BusRedisDBs >= test {
			t.Errorf("BusRedisDBs=%d but redis-test serves only 0..%d", testdb.BusRedisDBs, test-1)
		}
		if _, err := events.ClientOptions(fmt.Sprintf("localhost:1/%d", testdb.BusRedisDBs), ""); err != nil {
			t.Errorf("the highest bus database %d is refused as a bus address: %v", testdb.BusRedisDBs, err)
		}
	})

	// The file is what the container was asked for; this is what it serves.
	// They differ whenever a container outlives a change to the file.
	t.Run("the running test server serves that many", func(t *testing.T) {
		addr := testdb.RedisAddr(t)
		rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 0})
		defer closeProbe(t, rdb)
		res, err := rdb.ConfigGet(t.Context(), "databases").Result()
		if err != nil {
			t.Fatalf("reading the server's database count: %v", err)
		}
		if served := res["databases"]; served != strconv.Itoa(test) {
			t.Errorf("the server at %s serves %q databases but redis-test serves %d: recreate the container (`make db-down && make db-up`)",
				addr, served, test)
		}
	})
}

func TestALeaseIsTheTestsOwnUntilItEnds(t *testing.T) {
	t.Parallel()
	addr := testdb.RedisAddr(t)
	var first, second int
	key := "lease-probe:" + t.Name()
	t.Run("holder", func(t *testing.T) {
		t.Run("a bus lease", func(t *testing.T) {
			if bus := testdb.RedisBusDB(t); bus > testdb.BusRedisDBs {
				t.Errorf("a bus lease took db %d, above %d", bus, testdb.BusRedisDBs)
			}
		})
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
