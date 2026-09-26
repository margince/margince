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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AssertPoolsQuiesced fails the test if it ends while still holding a pooled
// connection. CALL IT where the pool is handed out; it registers its own
// cleanup.
//
// Closing the pool per test used to do this job by accident: a goroutine the test
// left running — a River client whose Stop timed out, a relay that outlived its
// context — started failing the moment its pool went away. A shared pool has no
// such moment, so that goroutine would go on claiming jobs and writing rows into
// the database the NEXT test just reset, and the wrong suite would report it.
//
// Register it in the fixture that hands out the pool, immediately, so it runs
// last: t.Cleanup is LIFO, and every cleanup a test adds later — the ones that
// stop the runners — must have already run before this can honestly claim the
// package is quiet.
//
// It waits on the POOL rather than sampling it, and that is the difference
// between a gate and a flake. A connection still checked out the instant cleanup
// runs is a race, not necessarily a leak: a goroutine that was asked to stop can
// be one round trip from releasing. Reading AcquiredConns() once failed real runs
// for exactly that, twice, on a loaded machine — different suites each time, which
// is the signature of a timing assumption rather than a defect. Taking every
// connection instead answers the question that matters — can anything else still
// be holding one — and gives a straggler that is finishing the time to finish,
// without asserting anything about the clock.
//
// PARALLEL SIBLINGS are why it counts holders rather than asserting on the spot.
// A t.Parallel() test's cleanup runs while its siblings are still running and
// legitimately holding connections from the same pool, so asserting there made
// the first test to finish report the rest as a leak — naming whichever test
// happened to end first, only on a loaded runner. The last holder out is the
// first moment the question "is anything still holding one" has an answer, so
// that is where it is asked.
//
// The cost is attribution, and it is the pool's rather than this gate's: with one
// pool and several tests running at once, nothing on the pgx side records which
// of them acquired what. So the failure names the group that shared it. A serial
// package is unaffected — one holder at a time, and the group is one name.
func AssertPoolsQuiesced(t *testing.T) {
	t.Helper()
	holdPool(t.Name())
	t.Cleanup(func() {
		group, last := releasePool()
		// A sibling is still running, and a connection it holds is its work
		// rather than this test's leak. Whoever finishes last asks for both.
		if !last {
			return
		}
		poolsMu.Lock()
		defer poolsMu.Unlock()
		for dsn, pool := range pools {
			assertPoolQuiesced(t, dsn, pool, group)
		}
	})
}

// sharing is every test that has taken the shared pool since the last time it
// went quiet, and finished is how many of them have ended. Guarded together
// because the gate reads them as one answer: the last holder out is the one
// that asserts, and it reports the group it was part of.
var (
	holdersMu sync.Mutex
	sharing   []string
	finished  int
)

// holdPool records that a test has taken the shared pool.
func holdPool(name string) {
	holdersMu.Lock()
	defer holdersMu.Unlock()
	sharing = append(sharing, name)
}

// releasePool records that a holder has finished, and reports the group it
// shared the pool with and whether it was the last of them out.
//
// The group RESETS once it empties, so a package's serial tests each report a
// group of one and a parallel batch reports itself. Without that, the first
// leak found would name every test the package had ever run.
func releasePool() ([]string, bool) {
	holdersMu.Lock()
	defer holdersMu.Unlock()
	finished++
	group := sharing
	if finished < len(sharing) {
		return group, false
	}
	sharing, finished = nil, 0
	return group, true
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

func assertPoolQuiesced(t *testing.T, dsn string, pool *pgxpool.Pool, group []string) {
	t.Helper()
	outstanding := poolQuiesced(pool, quiesceGrace, quiescePoll)
	if outstanding == 0 {
		return
	}
	// The group, not "this test": these may have run in parallel over one pool,
	// and the last one out is simply where the question could first be asked.
	blame := "this test"
	if len(group) > 1 {
		blame = "one of " + strings.Join(group, ", ")
	}
	t.Errorf("the shared pool for %s still had %d connection(s) checked out %s after %s ended. A goroutine it started is still running, and the next test resets the database under it: stop it in the test's own cleanup, which runs BEFORE this gate by design.",
		redactDSN(dsn), outstanding, quiesceGrace, blame)
}
