// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
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
// overdue past the window can still be edited, completed or snoozed. A decision
// task also keeps its review day at today or later.
func (in *UpdateActivityInput) admitPatchDueWindow(ctx context.Context, tx pgx.Tx, current crmcontracts.Activity) error {
	if !in.confirmedDue && movesDueDate(current.DueAt, in) {
		if err := checkDueWindow(in.DueAt, orWallClock(in.recordedAt)); err != nil {
			return err
		}
	}
	return in.admitPatchReviewDay(ctx, tx, current)
}

// ReviewDayPassedError refuses to move a decision's review to a day that is
// already over in the installation's calendar.
type ReviewDayPassedError struct{}

func (e *ReviewDayPassedError) Error() string { return "choose today or a later day" }

// FieldFault names due_at as the field to correct.
func (e *ReviewDayPassedError) FieldFault() (field, code, message string) {
	return fieldDueAt, "in_the_past", e.Error()
}

// admitPatchReviewDay keeps a recorded decision's review day at today or
// later in the installation's calendar, whichever door moves it.
func (in *UpdateActivityInput) admitPatchReviewDay(ctx context.Context, tx pgx.Tx, current crmcontracts.Activity) error {
	source := current.SourceSystem
	if source == nil || *source != provenance.AnalyticsDecisionSource || !movesDueDate(current.DueAt, in) || in.DueAt == nil {
		return nil
	}
	loc, err := resolveRecordZone(ctx, tx, in.recordZone)
	if err != nil {
		return err
	}
	if deadline.DayPassed(*in.DueAt, orWallClock(in.recordedAt), loc) {
		return &ReviewDayPassedError{}
	}
	return nil
}
