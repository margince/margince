// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ErrUnanswerable is a usage read on a meter that counts nothing it could
// report: one with no counter store, or one that bounds nothing.
var ErrUnanswerable = errors.New("agentvolume: this meter has no counters to report")

// CounterUsage is one allowance in the running window.
type CounterUsage struct {
	Used     int
	Limit    int
	Released int
}

// Usage is one subject's use of the four governed allowances in the running window.
type Usage struct {
	Reads, Writes, Egress, Calls CounterUsage
	ResetsAt                     time.Time
}

// UsageState is the worst of the four allowances.
type UsageState string

// The three states a Usage reports.
const (
	UsageOK      UsageState = "ok"
	UsageNear    UsageState = "near"
	UsageStopped UsageState = "stopped"
)

// nearShare is how much of an allowance is used before it reads as near.
const nearShare = 0.8

// State is stopped when any allowance is used up, near when any is at 80% or
// more, and ok otherwise.
func (u Usage) State() UsageState {
	state := UsageOK
	for _, c := range []CounterUsage{u.Reads, u.Writes, u.Egress, u.Calls} {
		switch {
		case c.Limit > 0 && c.Used >= c.Limit:
			return UsageStopped
		case c.Limit > 0 && float64(c.Used) >= nearShare*float64(c.Limit):
			state = UsageNear
		}
	}
	return state
}

// usageCounters is the order Usage reads the counters in, one count key and
// one release key each.
var usageCounters = []Counter{Reads, Writes, Egress, Calls}

// Usage reads the running window of every subject (SubjectOf) in one round
// trip. Like Release it names its subjects, because the caller is the human
// looking at them and not the agents being counted.
func (m *Meter) Usage(ctx context.Context, ws ids.UUID, subjects []string) (map[string]Usage, error) {
	if m.unbounded || m.rdb == nil {
		return nil, ErrUnanswerable
	}
	out := make(map[string]Usage, len(subjects))
	if len(subjects) == 0 {
		return out, nil
	}
	w := m.window(ctx)
	keys := make([]string, 0, 2*len(usageCounters)*len(subjects))
	for _, subject := range subjects {
		for _, c := range usageCounters {
			keys = append(keys, m.countKey(ws, subject, c, w.bucket), m.releaseKey(ws, subject, c, w.bucket))
		}
	}
	values, err := m.rdb.MGet(ctx, keys...).Result()
	m.noteReach(ignoreMissingKeys(err))
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("agentvolume: reading usage: %w", err)
	}
	if len(values) != len(keys) {
		return nil, fmt.Errorf("agentvolume: reading usage returned %d values, want %d", len(values), len(keys))
	}
	resetsAt := ResetAfter(m.now(), w.length)
	for i, subject := range subjects {
		usage := Usage{ResetsAt: resetsAt}
		for j, c := range usageCounters {
			slot := 2 * (i*len(usageCounters) + j)
			counter, err := w.counterUsage(c, values[slot], values[slot+1])
			if err != nil {
				return nil, err
			}
			*usage.of(c) = counter
		}
		out[subject] = usage
	}
	return out, nil
}

//craft:ignore naked-any go-redis MGet answers []any, so the slot's type is the client library's and not a choice made here
func (w window) counterUsage(c Counter, count, release any) (CounterUsage, error) {
	used, err := asCount(count)
	if err != nil {
		return CounterUsage{}, err
	}
	released, err := asCount(release)
	if err != nil {
		return CounterUsage{}, err
	}
	return CounterUsage{Used: used, Limit: w.effectiveLimit(c, released), Released: released}, nil
}

func (u *Usage) of(c Counter) *CounterUsage {
	switch c {
	case Reads:
		return &u.Reads
	case Writes:
		return &u.Writes
	case Egress:
		return &u.Egress
	case Calls:
		return &u.Calls
	default:
		panic(fmt.Sprintf("agentvolume: %s is not a counter Usage reports", c))
	}
}

// ResetsAt is when the running window resets.
func (m *Meter) ResetsAt(ctx context.Context) time.Time {
	return ResetAfter(m.now(), m.window(ctx).length)
}

// Plus sums two subjects' usage, for a reader that reports them as one. A
// user with two passports has used, and may use, what both have.
func (u Usage) Plus(o Usage) Usage {
	add := func(a, b CounterUsage) CounterUsage {
		return CounterUsage{Used: a.Used + b.Used, Limit: a.Limit + b.Limit, Released: a.Released + b.Released}
	}
	return Usage{
		Reads: add(u.Reads, o.Reads), Writes: add(u.Writes, o.Writes),
		Egress: add(u.Egress, o.Egress), Calls: add(u.Calls, o.Calls), ResetsAt: u.ResetsAt,
	}
}

var usageRank = map[UsageState]int{UsageOK: 0, UsageNear: 1, UsageStopped: 2}

// Worst is the more severe of two states.
func (s UsageState) Worst(o UsageState) UsageState {
	if usageRank[o] > usageRank[s] {
		return o
	}
	return s
}
