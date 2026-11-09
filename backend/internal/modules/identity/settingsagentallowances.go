// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How much one agent passport may do in one window. The meter reads these
// through a cache it refreshes each minute, so a new count binds in the running
// window. The window length changes only at a reset (agentvolume.WindowPlan).
// MachineryApplied because the meter applies them with no session to gate
// against.

import (
	"errors"
	"fmt"
	"math"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// AgentWritesPerWindow is the changing calls one passport may make per window.
var AgentWritesPerWindow = config.DefineStored(config.Spec[int]{
	Key:     "installation.agent_writes_per_window",
	Env:     "MARGINCE_INSTALLATION_AGENT_WRITES_PER_WINDOW",
	Type:    config.Int,
	Default: config.Some(agentvolume.DefaultWrites),
	Min:     config.Some(writesBounds.lowest),
	Max:     config.Some(writesBounds.highest),
	Doc:     "Changing calls one agent passport may make per window.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentEgressPerWindow is the calls leaving the workspace one passport may make per window.
var AgentEgressPerWindow = config.DefineStored(config.Spec[int]{
	Key:     "installation.agent_egress_per_window",
	Env:     "MARGINCE_INSTALLATION_AGENT_EGRESS_PER_WINDOW",
	Type:    config.Int,
	Default: config.Some(agentvolume.DefaultEgress),
	Min:     config.Some(egressBounds.lowest),
	Max:     config.Some(egressBounds.highest),
	Doc:     "Calls that send something out of the workspace one agent passport may make per window.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentCallsPerWindow is the calls of any kind, tool or API, one passport may make per window.
var AgentCallsPerWindow = config.DefineStored(config.Spec[int]{
	Key:     "installation.agent_calls_per_window",
	Env:     "MARGINCE_INSTALLATION_AGENT_CALLS_PER_WINDOW",
	Type:    config.Int,
	Default: config.Some(agentvolume.DefaultCalls),
	Min:     config.Some(callsBounds.lowest),
	Max:     config.Some(callsBounds.highest),
	Doc:     "Calls of any kind, tool or API, one agent passport may make per window.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentRoleWritesPerWindow is each role's own changing calls per window. The
// role maps are keyed by role key, which a rename leaves alone. A role absent
// from a map, or set to 0, uses the installation value.
var AgentRoleWritesPerWindow = config.DefineStored(config.Spec[map[string]int]{
	Key:      "installation.agent_role_writes_per_window",
	Env:      "MARGINCE_INSTALLATION_AGENT_ROLE_WRITES_PER_WINDOW",
	Type:     config.Map(config.Int),
	Validate: writesBounds.roleValues,
	Doc:      "Changing calls one agent passport may make per window, by the role key of the user who connected it, as role:count pairs; 0 uses the installation value. An agent gets the highest value of its user's roles; a role with no value counts as the installation value. Admin gets 2000 while neither Admin nor the installation count has a value set.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentRoleEgressPerWindow is each role's own calls leaving the workspace per window.
var AgentRoleEgressPerWindow = config.DefineStored(config.Spec[map[string]int]{
	Key:      "installation.agent_role_egress_per_window",
	Env:      "MARGINCE_INSTALLATION_AGENT_ROLE_EGRESS_PER_WINDOW",
	Type:     config.Map(config.Int),
	Validate: egressBounds.roleValues,
	Doc:      "Calls that send something out of the workspace one agent passport may make per window, by the role key of the user who connected it, as role:count pairs; 0 uses the installation value. An agent gets the highest value of its user's roles; a role with no value counts as the installation value.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentRoleCallsPerWindow is each role's own calls of any kind per window.
var AgentRoleCallsPerWindow = config.DefineStored(config.Spec[map[string]int]{
	Key:      "installation.agent_role_calls_per_window",
	Env:      "MARGINCE_INSTALLATION_AGENT_ROLE_CALLS_PER_WINDOW",
	Type:     config.Map(config.Int),
	Validate: callsBounds.roleValues,
	Doc:      "Calls of any kind, tool or API, one agent passport may make per window, by the role key of the user who connected it, as role:count pairs; 0 uses the installation value. An agent gets the highest value of its user's roles; a role with no value counts as the installation value. Admin gets 20000 while neither Admin nor the installation count has a value set.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// allowanceBounds is the range one count may be set to, for the installation
// and for a role alike.
type allowanceBounds struct{ lowest, highest int }

var (
	writesBounds = allowanceBounds{10, 5000}
	egressBounds = allowanceBounds{1, 200}
	callsBounds  = allowanceBounds{50, 20000}
)

// maxRoleAllowances bounds how many roles one map may name.
const maxRoleAllowances = 256

// roleValues holds every value of a role map to the installation's own range,
// or 0 for the installation value.
func (b allowanceBounds) roleValues(m map[string]int) error {
	if len(m) > maxRoleAllowances {
		return fmt.Errorf("at most %d roles may have a value of their own", maxRoleAllowances)
	}
	for role, value := range m {
		if role == "" {
			return errors.New("a role key cannot be empty")
		}
		if value != 0 && (value < b.lowest || value > b.highest) {
			return fmt.Errorf("a role's value is 0, for the installation value, or %d to %d", b.lowest, b.highest)
		}
	}
	return nil
}

// AgentWindowHours is the window length in force.
var AgentWindowHours = config.DefineStored(config.Spec[int]{
	Key:     "installation.agent_window_hours",
	Env:     "MARGINCE_INSTALLATION_AGENT_WINDOW_HOURS",
	Type:    config.Int,
	Default: config.Some(int(agentvolume.DefaultWindow.Hours())),
	Min:     config.Some(1),
	Max:     config.Some(168),
	Doc:     "Hours in the window the agent limits count over; set at boot, it also keeps a length an admin saved for the next reset from taking over.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentWindowNextHours is a saved length still waiting for its reset; 0 when
// none is. It and AgentWindowNextFrom are pending state the product writes,
// not configuration, so no boot source sets them.
var AgentWindowNextHours = settings.Define[int]("installation.agent_window_next_hours", installationSettingsObject, installationAuditVerb,
	0, settings.Between("hours", 0, 168)).MachineryApplied()

// AgentWindowNextFrom is when AgentWindowNextHours takes over, in Unix seconds;
// 0 when no change is waiting.
var AgentWindowNextFrom = settings.Define[int]("installation.agent_window_next_from", installationSettingsObject, installationAuditVerb,
	0, settings.Between("seconds", 0, math.MaxInt)).MachineryApplied()

func agentAllowanceSettings() []config.Any {
	return []config.Any{
		AgentWritesPerWindow, AgentEgressPerWindow, AgentCallsPerWindow,
		AgentRoleWritesPerWindow, AgentRoleEgressPerWindow, AgentRoleCallsPerWindow,
		AgentWindowHours,
	}
}
