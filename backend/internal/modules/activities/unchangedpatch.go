// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// changesNothing is true when every field the patch names already holds the
// sent value, and no clear names a field that holds one. That is a form saved
// untouched: writing it would bump the version and audit an empty change set.
func (in UpdateActivityInput) changesNothing(current crmcontracts.Activity) bool {
	return !in.movesAField(current) && !in.clearsAValue(current) && !in.changesCustom(current)
}

// movesAField compares each sent field with the row, the way a patch does.
func (in UpdateActivityInput) movesAField(current crmcontracts.Activity) bool {
	p := storekit.NewPatch()
	if in.Subject != nil {
		p.Set("subject", current.Subject, *in.Subject)
	}
	if in.Body != nil {
		p.Set("body", current.Body, *in.Body)
	}
	if in.OccurredAt != nil && !SameStoredStart(*in.OccurredAt, current.OccurredAt) {
		p.Set("occurred_at", current.OccurredAt, *in.OccurredAt)
	}
	if in.OccurredPrecision != nil {
		p.Set(fieldOccurredPrecision, string(current.OccurredPrecision), *in.OccurredPrecision)
	}
	if in.DurationSeconds != nil {
		p.Set("duration_seconds", current.DurationSeconds, *in.DurationSeconds)
	}
	if in.DueAt != nil {
		p.Set("due_at", current.DueAt, *in.DueAt)
	}
	if in.DueHasTime != nil {
		p.Set(fieldDueHasTime, current.DueHasTime, *in.DueHasTime)
	}
	if in.RemindAt != nil {
		p.Set("remind_at", current.RemindAt, *in.RemindAt)
	}
	if in.AssigneeID != nil {
		p.Set("assignee_id", current.AssigneeId, *in.AssigneeID)
	}
	if in.IsDone != nil {
		p.Set("is_done", current.IsDone, *in.IsDone)
	}
	if in.MeetingStatus != nil {
		p.Set("meeting_status", current.MeetingStatus, *in.MeetingStatus)
	}
	if in.CallOutcome != nil {
		p.Set(fieldCallOutcome, callOutcomeOf(current), *in.CallOutcome)
	}
	return !p.Empty()
}

// clearsAValue is true when a null names a field that currently holds one.
func (in UpdateActivityInput) clearsAValue(current crmcontracts.Activity) bool {
	return (in.clears("due_at") && current.DueAt != nil) ||
		(in.clears("remind_at") && current.RemindAt != nil) ||
		(in.clears("assignee_id") && current.AssigneeId != nil) ||
		(in.clearsDeclared(fieldCallOutcome, current) && current.CallOutcome != nil)
}
