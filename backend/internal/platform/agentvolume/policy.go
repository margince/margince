// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Policy is the allowances an installation set and the window they are counted over.
type Policy struct {
	Limits Limits
	Window WindowPlan
}

// PolicySource answers the installation's current Policy. It is a seam because
// the settings it reads belong to a module this package cannot import.
type PolicySource interface {
	Policy(ctx context.Context) (Policy, error)
}

// policyCache holds the last policy a source answered. Every meter rebound from
// one another shares it, so a process reads the settings at most once per
// PolicyRefresh.
type policyCache struct {
	mu       sync.Mutex
	held     Policy
	good     bool
	nextRead time.Time
	reading  bool
}

// policyRetry is how soon a failed read is tried again. It is sooner than a
// refresh, so a blip does not leave an admin's change unenforced for a minute.
const policyRetry = 5 * time.Second

// policyReadDeadline bounds one read of the source. The read is detached from its
// caller, so without this a hung database would hold the refresh forever.
const policyReadDeadline = 5 * time.Second

// WithPolicySource makes the meter judge every call by the source's policy.
// Set at assembly, before any request is served, like WithCostCeiling.
func (m *Meter) WithPolicySource(src PolicySource) *Meter {
	m.source, m.cache = src, &policyCache{}
	return m
}

// policy is the one policy a call is judged by.
//
// A failed read keeps the last good policy. With none, the meter's own
// limits bind, never an unbounded counter: withDefaults runs over whatever the
// source answered, so a zero cannot lift a ceiling.
//
// Once a policy is held, a refresh runs outside the lock and the calls meanwhile
// use the held one. Before that, calls wait for the first read, so none is
// judged by the defaults while an admin's values are on their way.
func (m *Meter) policy(ctx context.Context) Policy {
	if m.source == nil {
		return m.fallback
	}
	c := m.cache
	c.mu.Lock()
	now := m.now()
	if c.reading || now.Before(c.nextRead) {
		defer c.mu.Unlock()
		return c.heldOr(m.fallback)
	}
	// The caller's cancellation is dropped: one request giving up must not cost
	// every other call the policy.
	ask, cancel := context.WithTimeout(context.WithoutCancel(ctx), policyReadDeadline)
	defer cancel()
	if !c.good {
		defer c.mu.Unlock()
		p, err := m.source.Policy(ask)
		return m.record(p, err, now)
	}
	c.reading = true
	c.mu.Unlock()
	p, err := m.source.Policy(ask)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reading = false
	return m.record(p, err, now)
}

// record stores a read's outcome; the caller holds the cache lock.
func (m *Meter) record(p Policy, err error, now time.Time) Policy {
	c := m.cache
	if err != nil {
		slog.Warn("agent allowances could not be read; the last known allowances still bind", "err", err)
		c.nextRead = now.Add(policyRetry)
		return c.heldOr(m.fallback)
	}
	p.Limits = p.Limits.withDefaults()
	if p.Window.Length <= 0 {
		p.Window.Length = m.fallback.Window.Length
	}
	c.held, c.good, c.nextRead = p, true, now.Add(PolicyRefresh)
	return p
}

// ForgetPolicy makes the next call read the source again. A process that just
// saved the allowances calls it, so its own calls bind the new values at once;
// other processes take them within PolicyRefresh.
func (m *Meter) ForgetPolicy() {
	if m.cache == nil {
		return
	}
	m.cache.mu.Lock()
	defer m.cache.mu.Unlock()
	m.cache.nextRead = time.Time{}
}

func (c *policyCache) heldOr(fallback Policy) Policy {
	if c.good {
		return c.held
	}
	return fallback
}

// window is one call's view of the policy: the limits, the length in force and
// the bucket now falls in. They are taken together, so a call is never judged
// half by an old policy and half by a new one.
type window struct {
	limits Limits
	length time.Duration
	bucket int64
}

func (m *Meter) window(ctx context.Context) window {
	p := m.policy(ctx)
	now := m.now()
	length := p.Window.At(now)
	return window{limits: p.Limits, length: length, bucket: bucketAt(now, length)}
}

// effectiveLimit is the configured threshold plus what a human has released.
//
// One release grants one more allowance of the same size. That is the question
// the approval screen asked: the agent has read its window's worth, may it read
// another. A release that granted an unbounded continuation would make the
// second confirmation the last one anybody is ever asked for.
func (w window) effectiveLimit(c Counter, released int) int {
	base := w.limits.of(c)
	if released <= 0 {
		return base
	}
	return base * (released + 1)
}

// ttlSeconds covers the window plus an hour of clock-skew slack. So a counter
// never outlives its window, over-counting a later one, nor expires inside it,
// under-counting this one. Redis expiry has one-second granularity, so a
// sub-second window still gets a whole second at minimum.
func (w window) ttlSeconds() int {
	return max(1, int((w.length + time.Hour).Seconds()))
}

// reading is a Reading of counter c with nothing observed yet.
func (w window) reading(c Counter) Reading {
	return Reading{Counter: c, Limit: w.limits.of(c), Allowance: w.limits.of(c), Bucket: w.bucket, Window: w.length}
}
