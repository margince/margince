// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// AITaskDeclaration is one AI task a unit's api/ai-tasks.yaml fragment declares,
// read back out of the merged contract by gen-composition. It is re-emitted into
// the generated composition as a literal, the same path a JobDeclaration takes.
//
// The core registers it at boot, so Settings, AI shows the task by name with its
// own budget, call log and overrides. A unit calls it by Task, through
// Models.Complete. The core adds no prompt, no company context and no payload
// capture: the unit writes the whole request.
type AITaskDeclaration struct {
	// Unit is the declaring unit's directory name.
	Unit Name
	// Task is the contract key, ext_<unit namespace>_<task>. It is the name a unit
	// passes to Models.Complete and the name the call log shows.
	Task string
	// DisplayName and Summary are what Settings shows for the task.
	DisplayName string
	Summary     string
	// Ladder lists the tiers a call walks, best first, by their contract names.
	Ladder []string
	// Background is true for a task a job tick or a bus delivery may call. Such a
	// task queues at the budget limit. An interactive task degrades instead.
	Background bool
	// Sites are the named places in the unit that call the task.
	Sites []string
}
