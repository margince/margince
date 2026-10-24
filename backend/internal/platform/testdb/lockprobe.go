// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

// The one way this repository waits for a racer that is provably contended.
//
// Two suites had their own: approvals polls pg_stat_activity for a backend
// blocked by a known pid, and the stage-removal race polls FOR UPDATE NOWAIT
// until 55P03. Those are genuinely different QUESTIONS — "is anyone blocked by
// this pid" against "is this row held hard enough to refuse me" — and neither
// is redundant.
//
// What was duplicated is everything around the question: the budget, the
// pacing, the select over racer-finished / budget-expired / look-again, and the
// rule that a probe which gave up must say what the run failed to prove rather
// than pass having proved nothing. Two copies of that meant the reasoning could
// be corrected in one of them.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// ProbeBudget bounds the wait for a racer that never blocks.
//
// It is a DURATION, and it used to be a count of 20 000 probes. A count is not
// a budget: it is a race between how fast probe round trips complete and how
// fast the racer reaches its lock, and the lane's own concurrency slows BOTH,
// so a count generous on an idle machine is not generous on a loaded one. A
// duration means the same thing on every machine.
//
// Generous enough that only a genuine miss trips it, short enough that the miss
// reports itself rather than running into the package timeout, where it would
// read as a hung suite instead of a stated fact. That ceiling is arithmetic
// rather than taste: the approvals package alone has five call sites that can
// each spend this budget, against the lane's 600s per-package timeout
// (INTEGRATION_TIMEOUT). At 90s a run in which every one of them misses spends
// 450s and still reports what it found. Raise this number and that sum moves
// with it.
const ProbeBudget = 90 * time.Second

// ProbeInterval paces the poll, so the observer is not competing for the very
// resource it is waiting on.
//
// Unpaced, these loops issued round trips as fast as the server would answer
// them, and the pg_stat_activity probe's pg_blocking_pids is documented as
// needing exclusive access to the lock manager's shared state for a short time
// — the same state the racer must acquire to register its own lock wait. A
// watcher holding that thousands of times a second is not a neutral observer of
// contention; on a loaded runner it is part of it.
//
// 25ms is far finer than anything here needs: every block these tests wait for
// persists until the holding transaction ends, so it cannot be missed between
// ticks, and a racer that FINISHES is seen at once through racerReturned rather
// than on a tick.
const ProbeInterval = 25 * time.Millisecond

// WaitForContention polls look until it reports the contention the caller
// needs, the racer finishes without ever blocking, or the budget runs out.
//
// Both of the latter are failures, and each caller supplies what they mean in
// its own terms: a probe that gave up must say what the run failed to prove,
// never pass having proved nothing. finishedEarly is what a racer that returned
// without contending means for that test; missed is what a budget that expired
// with the racer still running means.
//
// look answers whether the contention is visible YET. An error it returns is
// fatal unless the budget has expired underneath it — at which point the
// failure is the budget's, not the query's, and reporting the query would name
// the wrong thing.
func WaitForContention(
	t *testing.T,
	racerReturned <-chan struct{},
	finishedEarly, missed string,
	look func(context.Context) (bool, error),
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), ProbeBudget)
	defer cancel()
	pace := time.NewTicker(ProbeInterval)
	defer pace.Stop()
	for {
		contended, err := look(ctx)
		switch {
		case err != nil && ctx.Err() != nil:
			t.Fatal(whichFailure(racerReturned, finishedEarly, missed))
		case err != nil:
			t.Fatalf("probing for a contended backend: %v", err)
		}
		if contended {
			return
		}
		// One select for all three answers: the racer finished, the budget ran
		// out, or it is time to look again.
		select {
		case <-racerReturned:
			t.Fatal(finishedEarly)
		case <-ctx.Done():
			t.Fatal(whichFailure(racerReturned, finishedEarly, missed))
		case <-pace.C:
		}
	}
}

// whichFailure picks which failure actually happened when the budget expires.
//
// When the racer finishes AS the budget runs out, both channels are ready and
// select picks between them arbitrarily — so the timeout branch can be taken
// while a finished racer sits unread, and the run would be reported as "nothing
// ever blocked" when what really happened is that the racer never blocked at
// all. Those are different diagnoses and the second one is the useful one.
func whichFailure(racerReturned <-chan struct{}, finishedEarly, missed string) string {
	select {
	case <-racerReturned:
		return finishedEarly
	default:
		return missed
	}
}

// QueuedBehind counts the sessions waiting on holder's backend, directly or
// behind one that does, since a lock queues its second waiter behind the first.
func QueuedBehind(ctx context.Context, conn *pgx.Conn, holder int32) (int, error) {
	if _, err := conn.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
		return 0, fmt.Errorf("clearing the stats snapshot before probing the lock queue: %w", err)
	}
	var queued int
	if err := conn.QueryRow(ctx, `
		WITH RECURSIVE queued AS (
			SELECT pid FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))
			UNION
			SELECT a.pid FROM pg_stat_activity a JOIN queued q ON q.pid = ANY(pg_blocking_pids(a.pid)))
		SELECT count(*) FROM queued`, holder).Scan(&queued); err != nil {
		return 0, fmt.Errorf("probing the lock queue behind backend %d: %w", holder, err)
	}
	return queued, nil
}

// AwaitQueuedBehind returns once n sessions wait behind holder. Scoped to that
// backend, so no unrelated lock wait stands in for the racer the failures name.
func AwaitQueuedBehind(t *testing.T, conn *pgx.Conn, holder int32, n int, racerReturned <-chan struct{}, racer string) {
	t.Helper()
	WaitForContention(t, racerReturned,
		fmt.Sprintf("%s finished without waiting behind backend %d, so the race never happened", racer, holder),
		fmt.Sprintf("%s never had %d session(s) waiting behind backend %d within %s", racer, n, holder, ProbeBudget),
		func(ctx context.Context) (bool, error) {
			queued, err := QueuedBehind(ctx, conn, holder)
			return queued >= n, err
		})
}
