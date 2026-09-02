// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package agents owns the governed agent surface (Layer 1): the MCP
// tool registry, the admission gate (scope ∧ tier ∧ the read/full seat
// ceiling; per-agent volume budget is specified but not yet enforced), the
// approval flow, and the Surface-B reasoning loop. It reaches records only
// through the datasource seam.
//
// Tables owned: agent_run and runner_job — the scheduled runner's own
// execution state — plus agent_standing_grant, which records whether a rep
// has said yes to an agent working on their behalf and which passport
// carries that authority.
//
// Nothing ELSE is owned here, and that is the line worth stating: records
// belong to the domain modules (reached via the injected datasource
// provider) and staged actions to approvals (reached via the injected
// adapter). The tool surface holds no state of its own; what this module
// owns is how a run is scheduled, authorized and recorded.
//
// The grant RECORDS authority and never confers it. A passport is minted by
// one statement that binds on_behalf_of and granted_by to the same session
// user, so a rep's standing authority is always a credential they minted
// themselves — see internal/modules/identity.
//
// Imports shared + platform only; never a sibling module.
package agents
