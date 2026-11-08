// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
)

// toAgentAllowances reports the plan as it stands at now. A change that already
// took over reads as the length in force, not as one still waiting.
func toAgentAllowances(p agentvolume.Policy, roles []identity.RoleAllowance, now time.Time) crmcontracts.AgentAllowanceSettings {
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
			AdminWritesPerWindow: identity.AdminAllowancePreset.Writes,
			AdminCallsPerWindow:  identity.AdminAllowancePreset.Calls,
		},
		Roles:                   toRoleAllowances(p.Limits, roles),
		McpCallsPerMinute:       mcpCallsPerMinute,
		McpStreamOpensPerMinute: mcpStreamOpensPerMinute,
	}
	if p.Window.Pending(now) {
		hours, from := int(p.Window.Next.Hours()), p.Window.NextFrom
		out.AgentWindowNextHours, out.AgentWindowNextStartsAt = &hours, &from
	}
	return out
}

// toRoleAllowances reports what each role gives per count and where it comes
// from: the role's own value first, then the Admin preset, then the installation.
func toRoleAllowances(installation agentvolume.Limits, roles []identity.RoleAllowance) []crmcontracts.AgentRoleAllowance {
	out := make([]crmcontracts.AgentRoleAllowance, 0, len(roles))
	for _, r := range roles {
		value := func(c agentvolume.Counter) crmcontracts.AgentRoleAllowanceValue {
			switch {
			case r.Own.Of(c) > 0:
				return crmcontracts.AgentRoleAllowanceValue{Value: r.Own.Of(c), Source: crmcontracts.AgentRoleAllowanceValueSourceRole}
			case r.Preset.Of(c) > 0:
				return crmcontracts.AgentRoleAllowanceValue{Value: r.Preset.Of(c), Source: crmcontracts.AgentRoleAllowanceValueSourceAdminPreset}
			default:
				return crmcontracts.AgentRoleAllowanceValue{Value: installation.Of(c), Source: crmcontracts.AgentRoleAllowanceValueSourceInstallation}
			}
		}
		out = append(out, crmcontracts.AgentRoleAllowance{
			RoleKey: r.Key, RoleName: r.Name, System: r.System, HoldsPreset: r.HoldsPreset,
			AgentWritesPerWindow: value(agentvolume.Writes),
			AgentEgressPerWindow: value(agentvolume.Egress),
			AgentCallsPerWindow:  value(agentvolume.Calls),
		})
	}
	return out
}

// unsetCounters names the counter each unset property gives up.
var unsetCounters = map[crmcontracts.AgentRoleAllowanceSetUnset]agentvolume.Counter{
	crmcontracts.AgentRoleAllowanceSetUnsetAgentWritesPerWindow: agentvolume.Writes,
	crmcontracts.AgentRoleAllowanceSetUnsetAgentEgressPerWindow: agentvolume.Egress,
	crmcontracts.AgentRoleAllowanceSetUnsetAgentCallsPerWindow:  agentvolume.Calls,
}

// roleAllowanceSets carries a save of some roles' values onto the store's
// shape; nil leaves every role as it is.
func roleAllowanceSets(sets *[]crmcontracts.AgentRoleAllowanceSet) *[]identity.RoleAllowanceSet {
	if sets == nil {
		return nil
	}
	out := make([]identity.RoleAllowanceSet, 0, len(*sets))
	for _, set := range *sets {
		next := identity.RoleAllowanceSet{RoleKey: set.RoleKey, Set: map[agentvolume.Counter]int{}}
		for counter, value := range map[agentvolume.Counter]*int{
			agentvolume.Writes: set.AgentWritesPerWindow, agentvolume.Egress: set.AgentEgressPerWindow,
			agentvolume.Calls: set.AgentCallsPerWindow,
		} {
			if value != nil {
				next.Set[counter] = *value
			}
		}
		if set.Unset != nil {
			for _, property := range *set.Unset {
				next.Unset = append(next.Unset, unsetCounters[property])
			}
		}
		out = append(out, next)
	}
	return &out
}
