// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package decisionview reads a task back as the sales decision it records.
package decisionview

import (
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// OutcomeMarker separates the expected result from the recorded one inside the
// task's body. The two are one text on the task, as a rep reads it.
const OutcomeMarker = "\n\nOutcome: "

// Of reads a task back as the decision it records.
func Of(task crmcontracts.Activity, loc *time.Location) crmcontracts.SalesDecision {
	out := crmcontracts.SalesDecision{Id: task.Id, Status: crmcontracts.SalesDecisionStatusOpen, CreatedAt: task.CreatedAt, OwnerId: task.AssigneeId, DoneAt: task.DoneAt}
	if task.Subject != nil {
		out.Decision = *task.Subject
	}
	if task.Links != nil {
		for _, link := range *task.Links {
			if link.EntityType == "deal" {
				out.DealId = link.EntityId
				break
			}
		}
	}
	out.ReviewOn = DueDay(task.DueIntent, task.DueAt, loc)
	if task.Body != nil {
		expected, result := SplitOutcome(*task.Body)
		if expected != "" {
			out.ExpectedResult = &expected
		}
		if result != nil {
			out.Result = result
		}
	}
	if Done(task) {
		out.Status = crmcontracts.SalesDecisionStatusDone
	}
	return out
}

// Done says whether the decision task is completed.
func Done(task crmcontracts.Activity) bool {
	return task.IsDone != nil && *task.IsDone
}

// SplitOutcome reads the expected result and, after the marker, the
// recorded one.
func SplitOutcome(body string) (expected string, result *string) {
	before, after, found := strings.Cut(body, OutcomeMarker)
	if !found {
		return body, nil
	}
	return before, &after
}

// DueDay is the installation's calendar day a task is due on. It is the day of
// the chosen due clock, else the due moment read in loc.
func DueDay(intent *crmcontracts.IntentTime, due *time.Time, loc *time.Location) *openapi_types.Date {
	day := ""
	switch {
	case intent != nil:
		day = intent.WallClock[:min(len(intent.WallClock), len(time.DateOnly))]
	case due != nil:
		day = due.In(loc).Format(time.DateOnly)
	}
	parsed, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return nil
	}
	return &openapi_types.Date{Time: parsed}
}
