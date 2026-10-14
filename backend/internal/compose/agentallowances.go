// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
)

// toAgentAllowances reports the plan as it stands at now. A change that already
// took over reads as the length in force, not as one still waiting.
func toAgentAllowances(p agentvolume.Policy, now time.Time) crmcontracts.AgentAllowanceSettings {
	current := p.Window.At(now)
	out := crmcontracts.AgentAllowanceSettings{
		AgentWritesPerWindow: p.Limits.Writes,
		AgentEgressPerWindow: p.Limits.Egress,
		AgentCallsPerWindow:  p.Limits.Calls,
		AgentWindowHours:     int(current.Hours()),
		AgentWindowResetsAt:  agentvolume.ResetAfter(now, current),
		Defaults: crmcontracts.AgentAllowanceDefaults{
			AgentWritesPerWindow: agentvolume.DefaultWrites,
			AgentEgressPerWindow: agentvolume.DefaultEgress,
			AgentCallsPerWindow:  agentvolume.DefaultCalls,
			AgentWindowHours:     int(agentvolume.DefaultWindow.Hours()),
		},
		McpCallsPerMinute:       mcpCallsPerMinute,
		McpStreamOpensPerMinute: mcpStreamOpensPerMinute,
	}
	if p.Window.Pending(now) {
		hours, from := int(p.Window.Next.Hours()), p.Window.NextFrom
		out.AgentWindowNextHours, out.AgentWindowNextStartsAt = &hours, &from
	}
	return out
}
