// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"
	"time"
)

// A task's due date lives between these years around now. A task far outside
// the window sits on no list, so a year typo would lose it without a word.
const (
	dueYearsBack  = 1
	dueYearsAhead = 10
)

// DueOutOfRangeError refuses a due date outside the window, naming due_at on
// every surface.
type DueOutOfRangeError struct{}

func (e *DueOutOfRangeError) Error() string {
	return fmt.Sprintf("due_at is more than %d year back or %d years ahead", dueYearsBack, dueYearsAhead)
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
	if due.Before(now.AddDate(-dueYearsBack, 0, 0)) || due.After(now.AddDate(dueYearsAhead, 0, 0)) {
		return &DueOutOfRangeError{}
	}
	return nil
}
