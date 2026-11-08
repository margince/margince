// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"slices"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/httperr"
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
// from. The order is the role's own value, a kept installation value, the
// Admin preset, then the installation value it inherits.
func toRoleAllowances(installation agentvolume.Limits, roles []identity.RoleAllowance) []crmcontracts.AgentRoleAllowance {
	out := make([]crmcontracts.AgentRoleAllowance, 0, len(roles))
	for _, r := range roles {
		value := func(c agentvolume.Counter) crmcontracts.AgentRoleAllowanceValue {
			out := crmcontracts.AgentRoleAllowanceValue{IfUnset: ifUnset(installation, r, c)}
			switch {
			case r.Own.Of(c) > 0:
				out.Value, out.Source = r.Own.Of(c), crmcontracts.AgentRoleAllowanceValueSourceRole
			case slices.Contains(r.Kept, c):
				out.Value, out.Source = installation.Of(c), crmcontracts.AgentRoleAllowanceValueSourceInstallationKept
			case r.Preset.Of(c) > 0:
				out.Value, out.Source = r.Preset.Of(c), crmcontracts.AgentRoleAllowanceValueSourceAdminPreset
			default:
				out.Value, out.Source = installation.Of(c), crmcontracts.AgentRoleAllowanceValueSourceInstallation
			}
			return out
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

// ifUnset is what a count gives once the role's own value is given up: the
// preset where it would bind, else the installation value.
func ifUnset(installation agentvolume.Limits, r identity.RoleAllowance, c agentvolume.Counter) crmcontracts.AgentRoleAllowanceFallback {
	if preset := r.PresetIfUnset.Of(c); preset > 0 {
		return crmcontracts.AgentRoleAllowanceFallback{Value: preset, Source: crmcontracts.AgentRoleAllowanceFallbackSourceAdminPreset}
	}
	return crmcontracts.AgentRoleAllowanceFallback{Value: installation.Of(c), Source: crmcontracts.AgentRoleAllowanceFallbackSourceInstallation}
}

// unsetCounters names the counter each unset property gives up.
var unsetCounters = map[crmcontracts.AgentRoleAllowanceSetUnset]agentvolume.Counter{
	crmcontracts.AgentRoleAllowanceSetUnsetAgentWritesPerWindow: agentvolume.Writes,
	crmcontracts.AgentRoleAllowanceSetUnsetAgentEgressPerWindow: agentvolume.Egress,
	crmcontracts.AgentRoleAllowanceSetUnsetAgentCallsPerWindow:  agentvolume.Calls,
}

// roleAllowanceSets carries a save of some roles' values onto the store's
// shape. An unset naming no count is refused, since dropping it would answer
// 200 for a change never made.
func roleAllowanceSets(sets []crmcontracts.AgentRoleAllowanceSet) ([]identity.RoleAllowanceSet, error) {
	out := make([]identity.RoleAllowanceSet, 0, len(sets))
	for _, set := range sets {
		next := identity.RoleAllowanceSet{RoleKey: set.RoleKey, Set: map[agentvolume.Counter]int{}}
		for counter, value := range map[agentvolume.Counter]*int{
			agentvolume.Writes: set.AgentWritesPerWindow, agentvolume.Egress: set.AgentEgressPerWindow,
			agentvolume.Calls: set.AgentCallsPerWindow,
		} {
			if value != nil {
				next.Set[counter] = *value
			}
		}
		var unset []crmcontracts.AgentRoleAllowanceSetUnset
		if set.Unset != nil {
			unset = *set.Unset
		}
		for _, property := range unset {
			counter, known := unsetCounters[property]
			if !known {
				return nil, httperr.Validation("agent_role_allowances", "invalid",
					fmt.Sprintf("unset names %q, which is not a count; name agent_writes_per_window, agent_egress_per_window or agent_calls_per_window", property))
			}
			next.Unset = append(next.Unset, counter)
		}
		out = append(out, next)
	}
	return out, nil
}
