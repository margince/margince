// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// DueOutOfRangeError refuses a due date outside the window, naming due_at on
// every surface.
type DueOutOfRangeError struct{}

func (e *DueOutOfRangeError) Error() string {
	return fmt.Sprintf("due_at is more than %d years back or %d years ahead", values.DueYearsBack, values.DueYearsAhead)
}

// FieldFault names due_at as the field to correct.
func (e *DueOutOfRangeError) FieldFault() (field, code, message string) {
	return fieldDueAt, codeOutOfRange, e.Error()
}

// checkDueWindow refuses a due moment outside the window. A nil due passes.
func checkDueWindow(due *time.Time, now time.Time) error {
	if due == nil {
		return nil
	}
	if !values.DueInWindow(*due, now) {
		return &DueOutOfRangeError{}
	}
	return nil
}

// admitPatchDueWindow checks only a due date the patch moves, so a task already
// overdue past the window can still be edited, completed or snoozed.
func (in *UpdateActivityInput) admitPatchDueWindow(current crmcontracts.Activity) error {
	if !movesDueDate(current.DueAt, in) {
		return nil
	}
	return checkDueWindow(in.DueAt, orWallClock(in.recordedAt))
}
