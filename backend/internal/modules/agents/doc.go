// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package agents owns the governed agent surface (Layer 1): the MCP
// tool registry, the approval flow, and the Surface-B reasoning loop. Every
// tool call passes the platform/auth admission gate: scope ∧ seat ∧ tier ∧ volume.
// It reaches records only through the datasource seam.
//
// Tables owned: docs/reference/entity-model/agents.md. agent_run and runner_job
// are the scheduled runner's own execution state. agent_standing_grant records
// whether a rep has said yes to an agent working for them. It also names the
// passport that carries that authority.
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
