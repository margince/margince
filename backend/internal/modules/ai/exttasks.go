// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The AI tasks an extension declares in api/ai-tasks.yaml.
//
// The core tasks are generated into tables at build time (tasks_gen.go). An
// extension task is known only when the installation is composed, so the boot
// adds it to the same tables once, before any surface serves. After that the
// router, the budget, the call log and the Settings lists treat it like a core
// task. AllTasks stays core-only, so certification does not walk it. RoutedTasks
// adds the extension tasks for the lists that show what the router serves.

import (
	"fmt"
	"slices"
	"sync"
)

// ExtensionTask is one task a unit declares, as the boot registers it.
type ExtensionTask struct {
	Task        Task
	Unit        string
	DisplayName string
	Summary     string
	Ladder      []Tier
	// Background tasks queue at the budget limit and may be called by a job tick
	// or a bus delivery. Interactive tasks degrade.
	Background bool
	Sites      []string
}

var extensionTasks = struct {
	mu    sync.RWMutex
	units map[Task]string
}{units: map[Task]string{}}

// RegisterExtensionTasks adds the declared tasks to the router's tables. It
// checks every task first and applies none if one is refused.
//
// It writes the generated tables, which the router reads without a lock, so it
// runs once at boot before any call is served.
func RegisterExtensionTasks(defs []ExtensionTask) error {
	for _, def := range defs {
		if err := def.validate(); err != nil {
			return err
		}
	}
	extensionTasks.mu.Lock()
	defer extensionTasks.mu.Unlock()
	for _, def := range defs {
		def.apply()
		extensionTasks.units[def.Task] = def.Unit
	}
	return nil
}

func (e ExtensionTask) validate() error {
	extensionTasks.mu.RLock()
	owner, registered := extensionTasks.units[e.Task]
	extensionTasks.mu.RUnlock()
	if _, core := taskLadders[e.Task]; core && !registered {
		return fmt.Errorf("ai: extension task %q is already a core task", e.Task)
	}
	if registered && owner != e.Unit {
		return fmt.Errorf("ai: extension task %q is declared by %s and by %s", e.Task, owner, e.Unit)
	}
	if e.DisplayName == "" || e.Summary == "" {
		return fmt.Errorf("ai: extension task %q needs a display name and a summary", e.Task)
	}
	if len(e.Ladder) == 0 {
		return fmt.Errorf("ai: extension task %q declares no tier", e.Task)
	}
	for _, tier := range e.Ladder {
		if !knownTiers[tier] {
			return fmt.Errorf("ai: extension task %q names unknown tier %q", e.Task, tier)
		}
	}
	if len(e.Sites) == 0 {
		return fmt.Errorf("ai: extension task %q declares no site", e.Task)
	}
	return nil
}

func (e ExtensionTask) apply() {
	mode := ExecutionModeInteractive
	if e.Background {
		mode = ExecutionModeBackground
	}
	sites := make([]Site, 0, len(e.Sites))
	for _, name := range e.Sites {
		sites = append(sites, Site{Name: name, Kind: SiteKindOneShot})
	}
	taskLadders[e.Task] = slices.Clone(e.Ladder)
	taskExecutionModes[e.Task] = mode
	taskDisplayNames[e.Task] = e.DisplayName
	taskSummaries[e.Task] = e.Summary
	taskStatus[e.Task] = StatusShipped
	taskSites[e.Task] = sites
	noPayloadTasks[e.Task] = true
	taskSubjectRestriction[e.Task] = SubjectRestrictionPosture{Posture: "honour"}
	railOwners[e.Task] = SourceRouter
}

// RoutedTasks lists every task the router serves, sorted: the core tasks and the
// extension tasks registered at boot. Settings reads it. Certification reads
// AllTasks, which is the core tasks only.
func RoutedTasks() []Task {
	extensionTasks.mu.RLock()
	defer extensionTasks.mu.RUnlock()
	tasks := AllTasks()
	for task := range extensionTasks.units {
		tasks = append(tasks, task)
	}
	slices.Sort(tasks)
	return tasks
}

// ExtensionTaskUnit names the unit that declared the task. It reports false for
// a core task or an unknown one.
func ExtensionTaskUnit(task Task) (string, bool) {
	extensionTasks.mu.RLock()
	defer extensionTasks.mu.RUnlock()
	unit, ok := extensionTasks.units[task]
	return unit, ok
}

// IsBackground reports whether the task is a background task.
func IsBackground(task Task) bool { return taskExecutionModes[task] == ExecutionModeBackground }

// ResetExtensionTasks removes the registered extension tasks. Tests call it.
func ResetExtensionTasks() {
	extensionTasks.mu.Lock()
	defer extensionTasks.mu.Unlock()
	for task := range extensionTasks.units {
		delete(taskLadders, task)
		delete(taskExecutionModes, task)
		delete(taskDisplayNames, task)
		delete(taskSummaries, task)
		delete(taskStatus, task)
		delete(taskSites, task)
		delete(noPayloadTasks, task)
		delete(taskSubjectRestriction, task)
		delete(railOwners, task)
	}
	clear(extensionTasks.units)
}
