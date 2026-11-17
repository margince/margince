// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

// Whether a test ended while something it started was still holding a pooled
// connection, and who is entitled to ask.
//
// Split from pool.go, which hands the pools out: that file answers "which pool
// does this DSN get", and this one answers "is anything still using it". The
// two meet only at the `pools` map.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AssertPoolsQuiesced fails the test if it ends while still holding a pooled
// connection to its own database. Call it where the pool is handed out. Its
// cleanup runs after every cleanup the test adds later, and before the drop.
//
// A goroutine the test left running would otherwise lose its database mid-query
// and fail in whichever test runs then. Only the test's own database is asked,
// because its parallel siblings hold connections to theirs.
func AssertPoolsQuiesced(t *testing.T) {
	t.Helper()
	db, ok := lookup(t.Name())
	if !ok {
		t.Fatal("testdb: AssertPoolsQuiesced needs the test's database; call testdb.New or testdb.For first")
	}
	t.Cleanup(func() {
		poolsMu.Lock()
		own := map[string]*pgxpool.Pool{}
		for dsn, pool := range pools {
			if on, err := databaseName(dsn); err == nil && on == db.Name {
				own[dsn] = pool
			}
		}
		poolsMu.Unlock()
		for dsn, pool := range own {
			assertPoolQuiesced(t, dsn, pool)
		}
	})
}

// quiesceGrace bounds how long a finishing goroutine has to hand its connection
// back, and quiescePoll is how often that is re-checked.
//
// Two seconds, not ten: the grace is paid per pool per leaking test, and a leak
// that touches a few dozen tests would otherwise spend the package's whole
// go-test budget waiting — reporting a package timeout instead of the straggler,
// which loses the gate's output in exactly the case it fires.
const (
	quiesceGrace = 2 * time.Second
	quiescePoll  = 20 * time.Millisecond
)

// poolQuiesced waits for pool to have nothing checked out, and reports what was
// still out when it gave up. Zero means quiet.
//
// It OBSERVES rather than acquires, and that distinction is the whole gate.
// Acquiring looks like waiting for a connection to come back and is not: pgxpool
// hands out an idle connection if it has one and dials a new backend otherwise, so
// asking for as many as are outstanding is satisfied at once while the straggler
// keeps its own. Against MaxConns of 16, an acquire-based wait could only block
// when nine or more were leaked together — never the one or two a straggler holds.
// It also inverted priority: holding the free slots starved the shutdown it was
// waiting for, then blamed it for the stall.
//
// What it does NOT catch is a straggler holding nothing at any instant. A poll
// loop that acquires, queries and releases reads as quiet in between, and that is
// a real shape — a River client whose Stop timed out. Catching it means watching
// AcquireCount across a window, and a window costs its own duration on every one
// of the lane's two thousand tests rather than only the ones that leak. #770
// tracks doing it for free by comparing the count across the gap BETWEEN tests,
// which is time the lane already spends. The leak that has actually fired here
// held its connection, which is the class below.
func poolQuiesced(pool *pgxpool.Pool, grace, poll time.Duration) int32 {
	outstanding := pool.Stat().AcquiredConns()
	if outstanding == 0 {
		return 0
	}
	// Something is out. One reading cannot tell a goroutine a round trip from
	// releasing apart from one still working, so give it the grace and then report
	// whatever is left.
	deadline := time.After(grace)
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			return pool.Stat().AcquiredConns()
		case <-tick.C:
			if outstanding = pool.Stat().AcquiredConns(); outstanding == 0 {
				return 0
			}
		}
	}
}

func assertPoolQuiesced(t *testing.T, dsn string, pool *pgxpool.Pool) {
	t.Helper()
	outstanding := poolQuiesced(pool, quiesceGrace, quiescePoll)
	if outstanding == 0 {
		return
	}
	t.Errorf("the pool for %s still had %d connection(s) checked out %s after the test ended. A goroutine it started is still running, and its database is dropped next: stop it in the test's own cleanup, which runs before this check.",
		redactDSN(dsn), outstanding, quiesceGrace)
}
