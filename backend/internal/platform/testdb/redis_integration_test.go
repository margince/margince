// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb_test

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// How many logical databases exist, and who owns which, is stated in four
// places that cannot check each other: this package's RedisDBs, REDIS_DBS in
// the parallel runner, DEV_REDIS_DB_MIN/MAX in scripts/dev.sh, and --databases
// in the compose file.
//
// Drift is silent in the direction that matters, in two ways. A lane db the
// server does not serve fails the suite that drew it with `ERR DB index is out
// of range`, naming Redis rather than the mismatch. And a DEV_SLUG stack
// landing inside the lane's range is worse than that: the suite that owns the
// db FLUSHDBs it between tests, so a developer's running stack loses its
// streams mid-run while the suite reads that stack's events as its own. Both
// failures point anywhere except here.
//
// Paths are relative to this package, which is three levels under backend/.
const (
	laneRunnerPath = "../../../../scripts/test-integration-parallel.sh"
	composePath    = "../../../../infra/docker-compose.dev.yml"
	devScriptPath  = "../../../../scripts/dev.sh"
)

// The declarations this reads. Anchored to the assignment rather than any
// mention, so a comment naming the number cannot satisfy the gate.
var (
	laneRunnerDecl = regexp.MustCompile(`(?m)^REDIS_DBS=(\d+)$`)
	composeDecl    = regexp.MustCompile(`--databases",\s*"(\d+)"`)
	devMinDecl     = regexp.MustCompile(`(?m)^DEV_REDIS_DB_MIN=(\d+)$`)
	devMaxDecl     = regexp.MustCompile(`(?m)^DEV_REDIS_DB_MAX=(\d+)$`)
)

func TestRedisDBCountAgreesWithTheLaneAndTheServer(t *testing.T) {
	t.Run("the parallel runner assigns from the same range", func(t *testing.T) {
		if got := declaredInt(t, laneRunnerPath, laneRunnerDecl); got != testdb.RedisDBs {
			t.Errorf("REDIS_DBS=%d in %s but testdb.RedisDBs=%d — the runner would assign a db this package rejects, or reject one it would accept",
				got, laneRunnerPath, testdb.RedisDBs)
		}
	})

	// Three blocks, in order and touching: db 0 is bare `make dev`, 1..RedisDBs
	// is the lane, and DEV_REDIS_DB_MIN..MAX is the slugged dev stacks. The
	// server must serve every index any of them can name.
	t.Run("the slugged dev range starts above the lane", func(t *testing.T) {
		devMin := declaredInt(t, devScriptPath, devMinDecl)
		if devMin <= testdb.RedisDBs {
			t.Errorf("DEV_REDIS_DB_MIN=%d in %s but the lane owns 1..%d — a slugged stack inside the lane's range has its streams FLUSHDB'd mid-run by the suite that believes it owns that db",
				devMin, devScriptPath, testdb.RedisDBs)
		}
	})

	t.Run("the compose file provisions every block", func(t *testing.T) {
		devMax := declaredInt(t, devScriptPath, devMaxDecl)
		if got := declaredInt(t, composePath, composeDecl); got != devMax+1 {
			t.Errorf("--databases %d in %s but DEV_REDIS_DB_MAX=%d — Redis serves 0..databases-1, so it must be asked for %d for the highest slugged stack's db to exist",
				got, composePath, devMax, devMax+1)
		}
	})

	// The file is what the container was asked for; this is what it actually
	// serves. They differ whenever a container outlives a change to the file,
	// which is the case no amount of agreement between the two files can catch.
	t.Run("the running server serves that many", func(t *testing.T) {
		addr := os.Getenv("MARGINCE_TEST_REDIS")
		if addr == "" {
			t.Fatal("MARGINCE_TEST_REDIS not set — run `make db-up` (integration tests fail loudly, they never skip)")
		}
		// Db 0, which every server has, rather than the db the lane assigned:
		// go-redis SELECTs on the first command, so asking an under-provisioned
		// server about a db it does not serve fails the SELECT and reports a
		// Redis read error — naming Redis instead of the drift this test exists
		// to name. Db 0 is the one a running `make dev` owns and no test may
		// flush; CONFIG GET reads a server setting and touches no key in it.
		rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 0})
		t.Cleanup(func() {
			if err := rdb.Close(); err != nil {
				t.Errorf("closing redis: %v", err)
			}
		})
		ctx := context.Background()
		res, err := rdb.ConfigGet(ctx, "databases").Result()
		if err != nil {
			t.Fatalf("reading the server's database count: %v", err)
		}
		raw, ok := res["databases"]
		if !ok {
			t.Fatalf("the server reported no `databases` setting (got %v)", res)
		}
		served, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("the server reported databases=%q, which is not a number", raw)
		}
		devMax := declaredInt(t, devScriptPath, devMaxDecl)
		if served != devMax+1 {
			t.Errorf("the server at %s serves %d databases but the blocks reach db %d — recreate the container (`make db-down && make db-up`); a container started before the count was raised keeps the old one",
				addr, served, devMax)
		}
	})
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
