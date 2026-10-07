// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The waiting horizon, remembered for an hour rather than re-measured on every
// read that needs it.
//
// The measurement is a 95th percentile of first-response times over a year:
// the most expensive read the waiting lane, the hidden-backlog guardrail and
// the owed pass depend on, and one whose answer cannot move within the hour.
// It is whole days, three times a year-long percentile, floored at a
// fortnight; an hour of new answers is a rounding error inside that, and the
// queue that reads it is judged in days.
//
// IN PROCESS, on the store, rather than in a table the owed job refreshes.
// The horizon is the same number for every reader of a workspace — that is the
// argument waitingHorizonFor makes for measuring it ungated — so one cached
// value per workspace is exactly as correct as one measured per call, and it
// needs nothing a table would add: no migration, no writer that has to be
// running for readers to be right, no staleness rule for the reader of a row
// the job stopped refreshing. What it costs is one measurement per process per
// hour instead of one shared by all of them, which is a handful an hour.
//
// What it gives up is stated: a read now takes a horizon measured in a
// transaction up to an hour older than its own, rather than in the same
// snapshot as the rows it bounds. The guarantee that mattered there — one
// cutoff for every half of one reading, so a difference between two scans is
// never a difference between two clocks — still holds, because each reading
// asks once and uses that answer throughout.

import (
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// waitingHorizonTTL is how long a measured horizon is reused.
//
// An hour because the owed pass is hourly-ish and the number is days: long
// enough that the pass stops paying for the measurement on every run, short
// enough that an installation whose answering genuinely changed — or whose
// administrator added a colleague domain the spread excludes — is followed the
// same working day.
const waitingHorizonTTL = time.Hour

// waitingHorizonFallbackTTL is how long the compiled horizon, standing in for a
// measurement that ran out of time, is reused before the measurement is tried
// again. Minutes, not an hour: the stand-in is not this installation's answer,
// but a measurement that just failed is likely to fail again, and a read that
// pays the statement budget for it twice (once per page) gets nothing.
const waitingHorizonFallbackTTL = 5 * time.Minute

// horizonMemo is one workspace's remembered horizon: what it was, the asOf it
// was measured for, when (by the store's clock) it was measured, and for how
// long it holds.
type horizonMemo struct {
	days       int
	asOf       time.Time
	measuredAt time.Time
	ttl        time.Duration
}

// horizonCache remembers the measured horizon per workspace.
//
// Keyed by WORKSPACE and nothing else, deliberately: the horizon is an
// installation-wide figure (waitingHorizonFor), so a key naming the reader
// would only store the same number several times — and suggest it could differ.
//
// Nil-safe, so a store assembled without NewStore — the unit tests that build
// one bare to exercise a gate — measures every time, which is the behaviour
// before this cache existed rather than a panic.
type horizonCache struct {
	mu    sync.Mutex
	memos map[ids.WorkspaceID]horizonMemo
}

func newHorizonCache() *horizonCache {
	return &horizonCache{memos: map[ids.WorkspaceID]horizonMemo{}}
}

// lookup answers the remembered horizon when it is still good for this read.
//
// Good means BOTH: measured less than a TTL ago by the store's clock, and
// measured for an asOf within a TTL of this one. The second condition is not
// redundant with the first. A caller asking about another moment — a report
// over last quarter, a suite pinning its asOf — is asking a different question
// of a year-long window, and must not be handed today's answer because the
// wall clock has not moved. A clock that runs BACKWARDS (a suite re-pinning it)
// reads as a miss rather than as a very fresh entry.
func (c *horizonCache) lookup(ws ids.WorkspaceID, asOf, now time.Time) (int, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	memo, ok := c.memos[ws]
	if !ok {
		return 0, false
	}
	age := now.Sub(memo.measuredAt)
	if age < 0 || age >= memo.ttl {
		return 0, false
	}
	if skew := asOf.Sub(memo.asOf).Abs(); skew >= waitingHorizonTTL {
		return 0, false
	}
	return memo.days, true
}

// remember stores a freshly measured horizon, replacing whatever was there.
//
// Two readers that miss together both measure and both store; the later write
// wins and both values are correct. Measuring under the lock instead would
// serialise every waiting read in the process behind one slow statement, which
// is the cost this cache exists to remove.
//
// A fallback never displaces a measurement that is still good: a reader whose
// measurement timed out must not replace what a concurrent reader measured.
//
// ttl is how long the entry holds: waitingHorizonTTL for a measurement,
// waitingHorizonFallbackTTL for the compiled horizon that stood in for one.
func (c *horizonCache) remember(ws ids.WorkspaceID, asOf, now time.Time, days int, ttl time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if held, ok := c.memos[ws]; ok && ttl == waitingHorizonFallbackTTL && held.ttl == waitingHorizonTTL {
		if age := now.Sub(held.measuredAt); age >= 0 && age < held.ttl {
			return
		}
	}
	c.memos[ws] = horizonMemo{days: days, asOf: asOf, measuredAt: now, ttl: ttl}
}
