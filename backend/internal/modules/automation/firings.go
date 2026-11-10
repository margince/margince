// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import "github.com/margince/margince/backend/internal/shared/ports/workflow"

// firing is one handler HandleEvent runs on one event, with the event as that
// handler receives it.
type firing struct {
	handler workflow.Handler
	event   workflow.Event
	version int64
}

// firings lists what HandleEvent runs on ev: every system handler its type
// triggers, then one firing per enabled instance of every handler it triggers.
// PendingFor predicts from the same list, so the two cannot disagree.
func (e *WorkflowEngine) firings(ev workflow.Event, instances map[string][]automationInstance) (system, instanced []firing) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, h := range e.system {
		if h.Spec().Trigger.EventType == ev.Type {
			system = append(system, firing{handler: h, event: ev})
		}
	}
	for _, h := range e.handlers {
		if h.Spec().Trigger.EventType != ev.Type {
			continue
		}
		for _, inst := range instances[h.Spec().Name] {
			iev := ev
			iev.AutomationID = inst.id.UUID
			iev.OwnerID = inst.owner
			iev.Params = inst.params
			instanced = append(instanced, firing{handler: h, event: iev, version: inst.version})
		}
	}
	return system, instanced
}
