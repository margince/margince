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
	mu     sync.Mutex
	held   Policy
	good   bool
	readAt time.Time
	tried  bool
}

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
func (m *Meter) policy(ctx context.Context) Policy {
	if m.source == nil {
		return m.fallback
	}
	now := m.now()
	c := m.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tried && now.Sub(c.readAt) < PolicyRefresh {
		return c.heldOr(m.fallback)
	}
	c.tried, c.readAt = true, now
	p, err := m.source.Policy(ctx)
	if err != nil {
		slog.WarnContext(ctx, "agent allowances could not be read; the last known allowances still bind", "err", err)
		return c.heldOr(m.fallback)
	}
	p.Limits = p.Limits.withDefaults()
	if p.Window.Length <= 0 {
		p.Window.Length = m.fallback.Window.Length
	}
	c.held, c.good = p, true
	return p
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
