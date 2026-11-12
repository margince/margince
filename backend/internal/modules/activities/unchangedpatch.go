// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"reflect"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// changesNothing is true when every field the patch names already holds the
// sent value, and no clear names a field that holds one. That is a form saved
// untouched: writing it would bump the version and audit an empty change set.
func (in UpdateActivityInput) changesNothing(current crmcontracts.Activity) bool {
	return !in.movesAField(current) && !in.clearsAValue(current) && !in.changesCustom(current)
}

func (in UpdateActivityInput) movesAField(current crmcontracts.Activity) bool {
	p := storekit.NewPatch()
	if in.sourceActivityID != nil {
		p.Set("source_activity_id", current.SourceActivityId, *in.sourceActivityID)
	}
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

func (in UpdateActivityInput) clearsAValue(current crmcontracts.Activity) bool {
	return (in.clears("due_at") && current.DueAt != nil) ||
		(in.clears("remind_at") && current.RemindAt != nil) ||
		(in.clears("assignee_id") && current.AssigneeId != nil) ||
		(in.clearsDeclared(fieldCallOutcome, current) && current.CallOutcome != nil)
}

// asksOnlyForTheDoneItHolds is true when the patch sets is_done to true and
// nothing else, on a task that is already done. The state the caller asked for
// holds, so its version no longer matters: a background write that moved the
// version must not make Done fail.
func (in UpdateActivityInput) asksOnlyForTheDoneItHolds(current crmcontracts.Activity) bool {
	if !in.doneAtAnyVersion || in.IsDone == nil || !*in.IsDone || current.IsDone == nil || !*current.IsDone {
		return false
	}
	// Every exported field is read, so a field added later cannot ride along
	// on a stale version.
	patch := reflect.ValueOf(in)
	for i := range patch.NumField() {
		field := patch.Type().Field(i)
		switch {
		case !field.IsExported(), field.Name == "IsDone", field.Name == "IfVersion", field.Name == "Trail":
			continue
		case patch.Field(i).Kind() == reflect.Map || patch.Field(i).Kind() == reflect.Slice:
			if patch.Field(i).Len() > 0 {
				return false
			}
		case !patch.Field(i).IsZero():
			return false
		}
	}
	return true
}
