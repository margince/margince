// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// taskActions is what a task row offers. Every verb PATCHes the task, so a
// reader who may not change it is offered none.
func taskActions(task Task) []crmcontracts.AttentionItemActions {
	if task.ReadOnly {
		return []crmcontracts.AttentionItemActions{}
	}
	actions := []crmcontracts.AttentionItemActions{"complete", "snooze"}
	// Offered on every scope the task appears in: nobody holding it is a fact
	// about the task, not about the page reading it.
	if task.AssigneeID == nil {
		actions = append(actions, "take")
	}
	return actions
}
