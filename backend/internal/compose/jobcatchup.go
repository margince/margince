// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// When a periodic pass first runs after the scheduling leader starts (#11026).
// River keeps schedules in memory, so a restart would push a pass back by a
// whole interval. RunOnStart ran every pass at once instead, due or not. Now
// the first run comes one interval after the kind's last run. A delay fixed
// per kind keeps the overdue passes from starting in the same minute.

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/clocktime"
)

// startupSpread bounds the delay a pass's first run takes after a start. A
// pass shorter than this spreads over its own interval instead.
const startupSpread = 5 * time.Minute

// catchUpSchedule is a pass's schedule. Its first Next after being armed
// catches up from the last run; every later Next is the plain interval.
type catchUpSchedule struct {
	kind    string
	every   river.PeriodicSchedule
	lastRun time.Time // the newest attempted_at on record, zero when none
	spread  time.Duration
	armed   atomic.Bool
}

func newCatchUpSchedule(kind string, every river.PeriodicSchedule, lastRun time.Time, spread time.Duration) *catchUpSchedule {
	s := &catchUpSchedule{kind: kind, every: every, lastRun: lastRun, spread: spread}
	s.armed.Store(true)
	return s
}

// Next is River's question. River asks it once when the schedule is placed,
// and after that with the run time it last scheduled.
func (s *catchUpSchedule) Next(current time.Time) time.Time {
	if !s.armed.CompareAndSwap(true, false) {
		return s.every.Next(current)
	}
	if clock, ok := s.every.(clocktime.Schedule); ok {
		return s.firstClockRun(clock, current)
	}
	first := current
	if !s.lastRun.IsZero() {
		if next := s.every.Next(s.lastRun); next.After(first) {
			first = next
		}
	}
	return first.Add(startupDelay(s.kind, s.every.Next(current).Sub(current), s.spread))
}

// replayedSlotMargin tells River's placement, which asks Next with the moment
// it starts, from a stored past slot River has already enqueued.
const replayedSlotMargin = 5 * time.Second

// firstClockRun is a clock-time pass's first run after a start: at most one
// catch-up, for a slot missed while the worker was down. A pass that never ran,
// or whose last run is after its latest slot, waits for its next slot.
func (s *catchUpSchedule) firstClockRun(clock clocktime.Schedule, current time.Time) time.Time {
	now := clock.Now()
	if current.Before(now.Add(-replayedSlotMargin)) {
		// River restored a stored slot and enqueued it; that run is the catch-up.
		return clock.Next(current)
	}
	missed := !s.lastRun.IsZero() && !clock.SlotAfter(s.lastRun).After(now)
	if !missed {
		return clock.Next(current)
	}
	return current.Add(startupDelay(s.kind, clock.Next(current).Sub(current), s.spread))
}

// rearm makes the next Next catch up again. A schedule handed back to River
// after a cadence change is placed again, and is treated like a start.
func (s *catchUpSchedule) rearm() { s.armed.Store(true) }

// startupDelay is fixed per kind, so a restart spreads the passes the same way
// each time, and it never exceeds the pass's own interval.
func startupDelay(kind string, interval, spread time.Duration) time.Duration {
	limit := min(spread, interval)
	if limit <= 0 {
		return 0
	}
	// FNV-1a over the kind name, scaled to a fraction of the limit.
	h := uint64(14695981039346656037)
	for i := range len(kind) {
		h = (h ^ uint64(kind[i])) * 1099511628211
	}
	fraction := float64(h>>11) / float64(1<<53) // in [0, 1)
	return time.Duration(fraction * float64(limit))
}

// periodicJob builds one pass's River entry. A book with no run history keeps
// RunOnStart: the defaults and the books tests build have none. A clock-time
// pass never runs at start, so it always waits through a catch-up schedule.
func (b *ScheduleBook) periodicJob(
	kind, id string, every river.PeriodicSchedule, ctor river.PeriodicJobConstructor,
) (*river.PeriodicJob, *catchUpSchedule) {
	if b.lastRuns == nil && !runsAtClockTimes(every) {
		return river.NewPeriodicJob(every, ctor, &river.PeriodicJobOpts{ID: id, RunOnStart: true}), nil
	}
	s := newCatchUpSchedule(kind, every, b.lastRuns[kind], b.spread)
	return river.NewPeriodicJob(s, ctor, &river.PeriodicJobOpts{ID: id}), s
}

// CatchUpFrom gives the book each kind's last run, so the passes built from it
// catch up instead of running at once. Call it before the runner is built.
func (b *ScheduleBook) CatchUpFrom(lastRuns map[string]time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if lastRuns == nil {
		lastRuns = map[string]time.Time{}
	}
	b.lastRuns, b.spread = lastRuns, startupSpread
}

// ReadLastRuns is the newest attempt per job kind still on record. Finished
// rows are cleaned after a retention window, so a kind missing here counts as
// overdue, which is the safe answer.
func ReadLastRuns(ctx context.Context, pool *pgxpool.Pool) (map[string]time.Time, error) {
	// pool-exempt: reads the job queue's own table, which no audit row describes.
	rows, err := pool.Query(ctx,
		`SELECT kind, max(attempted_at) FROM river_job WHERE attempted_at IS NOT NULL GROUP BY kind`)
	if err != nil {
		return nil, fmt.Errorf("compose: reading when each pass last ran: %w", err)
	}
	defer rows.Close()
	runs := map[string]time.Time{}
	for rows.Next() {
		var kind string
		var at time.Time
		if err := rows.Scan(&kind, &at); err != nil {
			return nil, fmt.Errorf("compose: reading when each pass last ran: %w", err)
		}
		runs[kind] = at
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading when each pass last ran: %w", err)
	}
	return runs, nil
}
