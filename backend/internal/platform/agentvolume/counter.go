// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

// Which counter a call belongs to, what each one costs when it is crossed, and
// the thresholds themselves.

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Counter is one per-Passport volume budget. The string value is part of the Redis key,
// so it is the name the counter has had since it was written and not a display
// label to be improved.
type Counter string

// The counters a Passport is held to. A read hands the agent nothing it can
// change, so it spends a call and has no counter of its own.
const (
	// Writes counts mutating calls (MCP-SESS-WRITES).
	Writes Counter = "writes"
	// Egress counts calls that leave the workspace (MCP-SESS-EGRESS).
	Egress Counter = "egress"
	// Calls counts every admitted tool call (MCP-SESS-CALLS).
	Calls Counter = "calls"
	// Cost counts model tokens spent behind a tool (MCP-SESS-COST).
	Cost Counter = "cost"
)

// Releasable reports whether a human can hand this counter another allowance
// mid-window. That is a step the connecting human may take, against a stop only
// the window can end.
//
// Writes is a batch-confirm. It turns bulk activity into a gated, visible event
// and does not end it. Egress is the exfiltration endpoint, so resuming needs a
// new approved session. Calls is the suspension under which every other counter
// sits, so releasing it would release all of them at once.
//
// Cost is not releasable because it never refuses.
func (c Counter) Releasable() bool {
	return c == Writes
}

// Governed reports whether crossing this counter refuses anything at all. Cost
// is soft by the spec's own word — a session cannot consume a disproportionate
// share of the workspace AI budget, but the ladder that acts on that is the AI
// runtime's own budget guardrail, not a refusal on the tool surface.
func (c Counter) Governed() bool { return c != Cost }

// CounterFor names the volume budget one tool call is charged against — the DERIVED
// answer, taken once, so the half that refuses and the half that charges cannot
// disagree.
//
// It is derived from what the spec already makes the surface declare rather than
// from a list of tool names. BYO-LIM-9 says so outright: the egress row "is
// defined by the egress flag, not by an enumerated list of names — a new
// egress-flagged tool joins it without an edit here". A maintained list is a
// list the tool added next is missing from, and the tool added next is exactly
// the one whose volume nobody has thought about.
//
// EGRESS OUTRANKS WRITES, and the order matters: every egress tool on this
// surface also mutates (a send writes an activity), so asking "does it write?"
// first would charge send_email against the 200-call write volume budget and leave the
// 20-call egress volume budget — the tightest one, guarding the exfiltration endpoint —
// permanently unspent.
func CounterFor(spec mcp.ToolSpec) Counter {
	switch {
	case spec.Egress:
		return Egress
	case spec.ReadOnly():
		return Calls
	default:
		return Writes
	}
}

// The default thresholds, from the SaaS column of the rate-limit spec. An admin
// changes them in the installation settings, within each setting's floor and
// ceiling. These bind until a policy source answers.
const (
	// DefaultWrites is MCP-SESS-WRITES: the mutating calls before the rest of
	// the window's writes batch-confirm.
	DefaultWrites = 200
	// DefaultEgress is MCP-SESS-EGRESS: the calls that may leave the workspace
	// before sends hard-stop for the window.
	DefaultEgress = 20
	// DefaultCalls is MCP-SESS-CALLS: the total tool calls before the Passport
	// is suspended for the window.
	DefaultCalls = 1000
)

// DefaultWindow is the span one counter covers. The spec names these bounds
// after a session; ADR-0055 and ADR-0092 leave the Passport as the only thing
// both doors share, so a Passport's day is what "session" resolves to here.
//
// It is a FIXED window, not a rolling one: the bucket is the moment divided by
// the window, so every counter in an installation resets at the same instant
// rather than a credential's own anniversary. That is what an operator needs to
// know to answer "when does this agent read again" — and it is why a refusal
// says "when the window rolls" rather than naming a duration. The span itself is
// stated in the tool copy an agent reads, so the surface and the operator use
// the same words.
const DefaultWindow = 24 * time.Hour

// Limits is one installation's thresholds. A zero field takes the default above,
// so a caller configuring one counter does not silently unbound the other three.
type Limits struct {
	Writes int
	Egress int
	Calls  int
}

// withDefaults fills every unset threshold. A NEGATIVE value is treated as unset
// too: it can only come from a misparsed configuration, and the alternative
// reading — a ceiling below zero, which every counter exceeds on its first
// charge — would suspend an installation's whole agent surface on a typo.
func (l Limits) withDefaults() Limits {
	if l.Writes <= 0 {
		l.Writes = DefaultWrites
	}
	if l.Egress <= 0 {
		l.Egress = DefaultEgress
	}
	if l.Calls <= 0 {
		l.Calls = DefaultCalls
	}
	return l
}

// of answers the threshold for one counter. Cost is absent on purpose: its
// ceiling is a share of the workspace's own AI budget, resolved per call through
// CostCeiling rather than configured per deployment here.
func (l Limits) of(c Counter) int {
	switch c {
	case Writes:
		return l.Writes
	case Egress:
		return l.Egress
	case Calls:
		return l.Calls
	case Cost:
		return 0
	default:
		return 0
	}
}
