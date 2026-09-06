// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An agent seat holds no queue, so no task may be aimed at one.
//
// The seat is an Agent Runner identity, not a person: it opens no Worklist, and
// the task lane is read per human. Work assigned to it therefore leaves every
// queue at once — it reads as delegated and is in fact abandoned, which is the
// worst of the three states a task can be in because nothing shows it.
//
// Both doors are asserted. The patch door has asked since it was written; the
// create door had not, so a task could be MINTED onto a seat and only fail to
// move afterwards — after it had sat in nobody's list for however long it took
// somebody to notice.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedAgentSeat writes one agent identity, the row an Agent Runner signs as.
func seedAgentSeat(t *testing.T) ids.UserID {
	t.Helper()
	seat := ids.NewV7()
	if _, err := OwnerConn(t).Exec(t.Context(), `
		INSERT INTO app_user (id, email, display_name, status, is_agent)
		VALUES ($1, $2, 'Runner', 'active', true)`,
		seat, "agent-"+seat.String()+"@seat.test"); err != nil {
		t.Fatalf("seeding the agent seat: %v", err)
	}
	return ids.From[ids.UserKind](seat)
}

func TestATaskCannotBeCreatedOnAnAgentSeat(t *testing.T) {
	e := Setup(t)
	seat := seedAgentSeat(t)
	subject := "Chase the renewal"
	due := time.Now().Add(24 * time.Hour)

	_, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &seat, Source: "manual",
	})
	var refusal *activities.AgentAssigneeError
	if !errors.As(err, &refusal) {
		t.Fatalf("creating a task on an agent seat got %v, want the field refusal", err)
	}
	// The FIELD, so the caller is sent to the thing they have to change rather
	// than looking for a typo in an id that is perfectly real.
	if field, _, _ := refusal.FieldFault(); field != "assignee_id" {
		t.Errorf("the refusal names %q, want assignee_id", field)
	}
}

func TestATaskCannotBeReassignedToAnAgentSeat(t *testing.T) {
	e := Setup(t)
	seat := seedAgentSeat(t)
	subject := "Send the summary"
	due := time.Now().Add(24 * time.Hour)
	mine := ids.From[ids.UserKind](e.Rep1)

	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &mine, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the task: %v", err)
	}

	_, err = e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(task.Id)),
		activities.UpdateActivityInput{AssigneeID: &seat})
	var refusal *activities.AgentAssigneeError
	if !errors.As(err, &refusal) {
		t.Fatalf("moving a task onto an agent seat got %v, want the field refusal", err)
	}
}

// The admission case, over the same fixture: a PERSON still takes the work.
// Without it every assertion above would pass on a door that refused everyone.
func TestATaskStillReachesAPerson(t *testing.T) {
	e := Setup(t)
	subject := "Book the call"
	due := time.Now().Add(24 * time.Hour)
	mine := ids.From[ids.UserKind](e.Rep1)

	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &mine, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging a task for a person: %v", err)
	}
	if task.AssigneeId == nil || ids.UUID(*task.AssigneeId) != e.Rep1 {
		t.Errorf("assignee = %v, want the person it was written for", task.AssigneeId)
	}
}
