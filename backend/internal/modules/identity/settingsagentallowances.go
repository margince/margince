// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How much one agent passport may do in one window. The meter reads these
// through a cache it refreshes each minute, so a new count binds in the running
// window. The window length changes only at a reset (agentvolume.WindowPlan).

import (
	"math"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// AgentReadsPerWindow is the records one passport may be handed per window.
var AgentReadsPerWindow = settings.Define[int]("installation.agent_reads_per_window", installationSettingsObject, "update",
	agentvolume.DefaultReads, settings.Between("records", 100, 20000)).MachineryApplied()

// AgentWritesPerWindow is the changing calls one passport may make per window.
var AgentWritesPerWindow = settings.Define[int]("installation.agent_writes_per_window", installationSettingsObject, "update",
	agentvolume.DefaultWrites, settings.Between("calls", 10, 5000)).MachineryApplied()

// AgentEgressPerWindow is the calls leaving the workspace one passport may make per window.
var AgentEgressPerWindow = settings.Define[int]("installation.agent_egress_per_window", installationSettingsObject, "update",
	agentvolume.DefaultEgress, settings.Between("calls", 1, 200)).MachineryApplied()

// AgentCallsPerWindow is the tool calls of any kind one passport may make per window.
var AgentCallsPerWindow = settings.Define[int]("installation.agent_calls_per_window", installationSettingsObject, "update",
	agentvolume.DefaultCalls, settings.Between("calls", 50, 20000)).MachineryApplied()

// AgentWindowHours is the window length in force.
var AgentWindowHours = settings.Define[int]("installation.agent_window_hours", installationSettingsObject, "update",
	int(agentvolume.DefaultWindow.Hours()), settings.Between("hours", 1, 168)).MachineryApplied()

// AgentWindowNextHours is a saved length still waiting for its reset; 0 when
// none is.
var AgentWindowNextHours = settings.Define[int]("installation.agent_window_next_hours", installationSettingsObject, "update",
	0, settings.Between("hours", 0, 168)).MachineryApplied()

// AgentWindowNextFrom is when AgentWindowNextHours takes over, in Unix seconds;
// 0 when no change is waiting.
var AgentWindowNextFrom = settings.Define[int]("installation.agent_window_next_from", installationSettingsObject, "update",
	0, settings.Between("seconds", 0, math.MaxInt)).MachineryApplied()

func agentAllowanceDefinitions() []settings.Definition {
	return []settings.Definition{
		AgentReadsPerWindow, AgentWritesPerWindow, AgentEgressPerWindow, AgentCallsPerWindow,
		AgentWindowHours, AgentWindowNextHours, AgentWindowNextFrom,
	}
}
