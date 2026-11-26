// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// taskActions offers the changes permitted for this task. Claiming its unowned
// lead can make an inquiry task writable in the same transaction.
func taskActions(task Task) []crmcontracts.AttentionItemActions {
	actions := []crmcontracts.AttentionItemActions{}
	if !task.ReadOnly {
		actions = append(actions, "complete", "snooze")
	}
	if task.WebsiteInquiry {
		if task.Inquiry != nil && task.Inquiry.CanTake != nil && *task.Inquiry.CanTake {
			actions = append(actions, "take")
		}
	} else if !task.ReadOnly && task.AssigneeID == nil {
		actions = append(actions, "take")
	}
	return actions
}
