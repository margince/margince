// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package agentvolume is the per-Passport volume accounting the MCP session used
// to stand in for. It counts how much one agent has called, changed, sent and
// spent inside one window, and decides what happens when it passes a threshold.
//
// It is per Passport and not per session. The spec names these counters after a
// session, and the in-process session registry used to bound volume. That bound
// pinned an agent to one api replica and did not exist on the REST half of the
// same credential. ADR-0092 removes the registry and keeps the per-Passport
// counters as its replacement. The Passport is the one thing both doors share, so
// it is the key that survives the change.
//
// It counts calls, not records. `writes`, `egress` and `calls` each count one
// act whatever it touched. A read is a call too. It changes nothing, and a count
// that moved with the size of a page would read as a bug to whoever watches it.
// `cost` counts model tokens, the unit the workspace AI budget is denominated in.
//
// What happens at a threshold depends on the counter. `calls` and `egress` stop
// the agent for the window. `writes` asks the human who lent the Passport whether
// to continue. `cost` only says so. See Counter.
//
// It is fail-closed. With no Redis, or a Redis that errors, the meter cannot know
// whether a threshold has been passed. A control that cannot answer must not
// answer "no", so every governed counter reports the threshold as passed. Agent
// traffic stops while the counter is unreachable. Human sessions never enter
// this path, because their authority is RBAC at the store.
//
// It lives in the platform tier and owns no domain. Both doors onto the governed
// surface charge it, the MCP tool dispatcher and the REST agent gate, and
// neither may import the other.
package agentvolume
