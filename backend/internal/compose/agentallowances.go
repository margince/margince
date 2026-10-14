// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
)

// The per-minute bounds of the MCP transport. They guard the server against a
// flood and do not set what an agent may do. So the build fixes them, and the
// settings page shows them read-only.
const (
	mcpCallsPerMinute       = 240
	mcpStreamOpensPerMinute = 30
)

// toAgentAllowances reports the plan as it stands at now. A change that already
// took over reads as the length in force, not as one still waiting.
func toAgentAllowances(p agentvolume.Policy, now time.Time) crmcontracts.AgentAllowanceSettings {
	current := p.Window.At(now)
	out := crmcontracts.AgentAllowanceSettings{
		AgentReadsPerWindow:     p.Limits.Reads,
		AgentWritesPerWindow:    p.Limits.Writes,
		AgentEgressPerWindow:    p.Limits.Egress,
		AgentCallsPerWindow:     p.Limits.Calls,
		AgentWindowHours:        int(current.Hours()),
		AgentWindowResetsAt:     agentvolume.ResetAfter(now, current),
		McpCallsPerMinute:       mcpCallsPerMinute,
		McpStreamOpensPerMinute: mcpStreamOpensPerMinute,
	}
	if p.Window.Pending(now) {
		hours, from := int(p.Window.Next.Hours()), p.Window.NextFrom
		out.AgentWindowNextHours, out.AgentWindowNextStartsAt = &hours, &from
	}
	return out
}
