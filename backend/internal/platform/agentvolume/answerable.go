// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

// Whether this role's agent-read control can answer at all — the one fact an
// operator needs and could not see.
//
// The meter fails CLOSED: with its counter store unreachable every governed
// counter reports its threshold passed, and the whole agent surface starts
// refusing. That is the right answer to give a caller and the wrong thing to
// keep to itself. On the default api the fault is visible by accident — the
// inline relay probes the same Redis, so /readyz takes the pod out — but a
// SPLIT role, api with the relay off and the worker separate, probes nothing
// and reports healthy while every agent read refuses. Silent failure looks
// exactly like health.
//
// Readiness is deliberately NOT where this goes. It is per-pod, so a probe
// would take the pod out of rotation for human traffic too, over a fault no
// human request can meet — trading an agent-only outage for a total one. So
// the signal is a metric an operator alerts on, and the alert's own text says
// what it means: agent reads are refusing, human traffic is unaffected.

import "sync/atomic"

// Reachability is what this role can say about its agent-read control.
type Reachability struct {
	// Bound reports that this composition declared a bound at all. An
	// Unmetered meter answers false, and it is not a fault: that role was
	// never asked to bound an agent, and reporting it as unreachable would
	// alert an operator about a control nobody wanted.
	Bound bool
	// Reachable reports whether the last attempt to read the counter store
	// succeeded. True before the first attempt: a process that has served no
	// agent read has not failed to serve one, and starting at false would
	// alert every role at boot for as long as no agent connected.
	Reachable bool
}

// noteReach records the outcome of one attempt to reach the counter store.
//
// Last-attempt rather than a failure COUNT, because the question an operator
// asks is present tense — is the agent surface refusing right now — and a
// counter that has ever incremented cannot answer it. The recovery edge
// matters as much as the failing one: a meter that healed must stop alerting
// without anybody clearing anything.
func (m *Meter) noteReach(err error) {
	if m.reachable != nil {
		m.reachable.Store(err == nil)
	}
}

// Answerable reports whether this role's agent-read control can answer.
//
// A meter with no counter store at all is reported unreachable rather than
// unbound: it was asked to bound an agent and cannot, which is the fail-closed
// state this exists to make visible, and the difference between "no Redis
// configured" and "Redis went away" is not one an operator acts on differently.
func (m *Meter) Answerable() Reachability {
	// A nil meter is a composition that wired none — no bound was declared, so
	// there is nothing failing. Nil-safe because the caller is the /metrics
	// renderer, and a scrape that panics takes away the visibility this
	// exists to add.
	if m == nil || m.unbounded {
		return Reachability{}
	}
	return Reachability{Bound: true, Reachable: m.reachable != nil && m.reachable.Load()}
}

// reachableAtBoot is the flag a fresh meter carries, seeded from whether it has
// a store to reach at all — a meter composed without one has already failed to
// reach it, and there is no later attempt that will say so.
//
// A POINTER, because a Meter is copied field by field at assembly (RebindFrom)
// and an atomic value is not a thing to copy; sharing the flag is also correct
// — the rebound meter and the one it came from are the same control.
func reachableAtBoot(hasStore bool) *atomic.Bool {
	var b atomic.Bool
	// True before the first ATTEMPT, when there is something to attempt: see
	// Reachability.Reachable.
	b.Store(hasStore)
	return &b
}
