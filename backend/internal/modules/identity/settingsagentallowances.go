// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How much one agent passport may do in one window. The meter reads these
// through a cache it refreshes each minute, so a new count binds in the running
// window. The window length changes only at a reset (agentvolume.WindowPlan).
// MachineryApplied because the meter applies them with no session to gate
// against.

import (
	"math"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/config"
)

// AgentWritesPerWindow is the changing calls one passport may make per window.
var AgentWritesPerWindow = config.DefineStored(config.Spec[int]{
	Key:     "installation.agent_writes_per_window",
	Env:     "MARGINCE_INSTALLATION_AGENT_WRITES_PER_WINDOW",
	Type:    config.Int,
	Default: config.Some(agentvolume.DefaultWrites),
	Min:     config.Some(10),
	Max:     config.Some(5000),
	Doc:     "Changing calls one agent passport may make per window.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentEgressPerWindow is the calls leaving the workspace one passport may make per window.
var AgentEgressPerWindow = config.DefineStored(config.Spec[int]{
	Key:     "installation.agent_egress_per_window",
	Env:     "MARGINCE_INSTALLATION_AGENT_EGRESS_PER_WINDOW",
	Type:    config.Int,
	Default: config.Some(agentvolume.DefaultEgress),
	Min:     config.Some(1),
	Max:     config.Some(200),
	Doc:     "Calls that send something out of the workspace one agent passport may make per window.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentCallsPerWindow is the calls of any kind, tool or API, one passport may make per window.
var AgentCallsPerWindow = config.DefineStored(config.Spec[int]{
	Key:     "installation.agent_calls_per_window",
	Env:     "MARGINCE_INSTALLATION_AGENT_CALLS_PER_WINDOW",
	Type:    config.Int,
	Default: config.Some(agentvolume.DefaultCalls),
	Min:     config.Some(50),
	Max:     config.Some(20000),
	Doc:     "Calls of any kind, tool or API, one agent passport may make per window.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentWindowHours is the window length in force.
var AgentWindowHours = config.DefineStored(config.Spec[int]{
	Key:     "installation.agent_window_hours",
	Env:     "MARGINCE_INSTALLATION_AGENT_WINDOW_HOURS",
	Type:    config.Int,
	Default: config.Some(int(agentvolume.DefaultWindow.Hours())),
	Min:     config.Some(1),
	Max:     config.Some(168),
	Doc:     "Hours in the window the agent allowances count over. Set at boot, it also keeps a length an administrator saved for the next reset from taking over.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentWindowNextHours is a saved length still waiting for its reset; 0 when
// none is.
var AgentWindowNextHours = config.DefineStored(config.Spec[int]{
	Key:     "installation.agent_window_next_hours",
	Env:     "MARGINCE_INSTALLATION_AGENT_WINDOW_NEXT_HOURS",
	Type:    config.Int,
	Default: config.Some(0),
	Min:     config.Some(0),
	Max:     config.Some(168),
	Doc:     "The window length an administrator saved for the next reset, 0 when none waits. The product writes it.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// AgentWindowNextFrom is when AgentWindowNextHours takes over, in Unix seconds;
// 0 when no change is waiting.
var AgentWindowNextFrom = config.DefineStored(config.Spec[int]{
	Key:     "installation.agent_window_next_from",
	Env:     "MARGINCE_INSTALLATION_AGENT_WINDOW_NEXT_FROM",
	Type:    config.Int,
	Default: config.Some(0),
	Min:     config.Some(0),
	Max:     config.Some(math.MaxInt),
	Doc:     "When the saved window length takes over, in Unix seconds, 0 when none waits. The product writes it.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

func agentAllowanceSettings() []config.Any {
	return []config.Any{
		AgentWritesPerWindow, AgentEgressPerWindow, AgentCallsPerWindow,
		AgentWindowHours, AgentWindowNextHours, AgentWindowNextFrom,
	}
}
