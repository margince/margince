// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"github.com/margince/margince/backend/internal/compose/momentaction"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Sourced names the tasks read out of a message, the only ones a request
// reminder can be.
func Sourced(tasks []Task) []ids.UUID {
	var named []ids.UUID
	for _, task := range tasks {
		if task.SourceActivityID != nil {
			named = append(named, task.ID)
		}
	}
	return named
}

// DoneCandidates are the promise tasks with a source message, each with its
// open recipients.
func DoneCandidates(tasks []Task) []momentaction.DoneCandidate {
	var out []momentaction.DoneCandidate
	for _, task := range tasks {
		if !task.Commitment || task.SourceActivityID == nil {
			continue
		}
		candidate := momentaction.DoneCandidate{TaskID: task.ID, SourceActivityID: *task.SourceActivityID, Assignee: task.AssigneeID}
		for _, recipient := range task.Recipients {
			if momentaction.OpenRecipient(string(recipient.Status)) {
				candidate.Recipients = append(candidate.Recipients, ids.UUID(recipient.ContactId))
			}
		}
		out = append(out, candidate)
	}
	return out
}
