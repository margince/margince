// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// fieldDueHasTime is the flag that says a task's due_at names a time of day.
const fieldDueHasTime = "due_has_time"

// admitTaskTime settles a new task's time of day. The flag means something only
// beside a due_at, and only on a task. A task with a time reminds its assignee
// at that time unless the writer named a reminder of its own. That rule lives
// here, under every door that creates a task (REST, the agent tools, a
// follow-up, an accepted request), and not in any one form.
func admitTaskTime(in *LogActivityInput) error {
	if !in.DueHasTime {
		return nil
	}
	if in.Kind != string(crmcontracts.ActivityKindTask) {
		return &KindFieldError{Field: fieldDueHasTime, Only: "a task"}
	}
	if in.DueAt == nil {
		return &RequiredFieldError{Field: fieldDueAt}
	}
	if in.RemindAt == nil {
		due := *in.DueAt
		in.RemindAt = &due
	}
	return nil
}

// admitPatchTime pairs the flag with due_at on a patch. A due_at sent without
// the flag is a day with no time, so an older client moving a timed task does
// not leave the old time standing on a new day. The flag alone is refused:
// the time of day it would vouch for is due_at's.
func (in *UpdateActivityInput) admitPatchTime(kind crmcontracts.ActivityKind) error {
	if in.DueHasTime != nil && kind != crmcontracts.ActivityKindTask {
		return &KindFieldError{Field: fieldDueHasTime, Only: "a task"}
	}
	if in.DueHasTime != nil && in.DueAt == nil {
		return &RequiredFieldError{Field: fieldDueAt}
	}
	if in.DueAt != nil && in.DueHasTime == nil {
		in.DueHasTime = new(false)
	}
	return nil
}

// followReminder keeps a task's reminder on its agreed time, for every caller
// of the update. A move to a time sets the reminder to that time. A move to a
// day with no time, or a cleared due date, ends a reminder that sat on the old
// time. A patch that names remind_at itself is left alone.
//
// It runs before endPassedReminder, so a passed reminder on a task moved to a
// new time is set again rather than ended.
func (in *UpdateActivityInput) followReminder(current crmcontracts.Activity) {
	if in.RemindAt != nil || in.clears("remind_at") {
		return
	}
	if in.DueAt != nil && in.DueHasTime != nil && *in.DueHasTime {
		due := *in.DueAt
		in.RemindAt = &due
		return
	}
	moved := in.DueAt != nil || in.clears("due_at")
	satOnTime := current.DueHasTime != nil && *current.DueHasTime && current.RemindAt != nil &&
		current.DueAt != nil && current.RemindAt.Equal(*current.DueAt)
	if moved && satOnTime {
		in.Clear = append(in.Clear, "remind_at")
	}
}
