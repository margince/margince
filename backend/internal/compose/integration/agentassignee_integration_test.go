// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An agent seat holds no queue, so no task may be aimed at one.
//
// The seat is an Agent Runner identity, not a contact: it opens no Worklist, and
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
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
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
	assertSeatRefused(t, err, "assignee_id")
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
	assertSeatRefused(t, err, "assignee_id")
}

// The admission case, over the same fixture: a CONTACT still takes the work.
// Without it every assertion above would pass on a door that refused everyone.
func TestATaskStillReachesAContact(t *testing.T) {
	e := Setup(t)
	subject := "Book the call"
	due := time.Now().Add(24 * time.Hour)
	mine := ids.From[ids.UserKind](e.Rep1)

	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &mine, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging a task for a contact: %v", err)
	}
	if task.AssigneeId == nil || ids.UUID(*task.AssigneeId) != e.Rep1 {
		t.Errorf("assignee = %v, want the contact it was written for", task.AssigneeId)
	}
}

// A seat that cannot hold work gets one answer on every door: 422 on the field
// the caller sent. An unknown id, a retired colleague and an agent read alike,
// so the answer discloses nothing about the seat.
func TestARefusedSeatGetsOneAnswerOnEveryDoor(t *testing.T) {
	e := Setup(t)
	retired := ids.NewV7()
	if _, err := OwnerConn(t).Exec(t.Context(), `
		INSERT INTO app_user (id, email, display_name, status) VALUES ($1, $2, 'Gone', 'deactivated')`,
		retired, "gone-"+retired.String()+"@seat.test"); err != nil {
		t.Fatalf("seeding the retired seat: %v", err)
	}
	reader := ids.NewV7()
	if _, err := OwnerConn(t).Exec(t.Context(), `
		INSERT INTO app_user (id, email, display_name, status, seat_type) VALUES ($1, $2, 'Reader', 'active', 'read')`,
		reader, "reader-"+reader.String()+"@seat.test"); err != nil {
		t.Fatalf("seeding the read seat: %v", err)
	}
	seats := map[string]ids.UserID{
		"a read seat":         ids.From[ids.UserKind](reader),
		"an agent seat":       seedAgentSeat(t),
		"a retired colleague": ids.From[ids.UserKind](retired),
		"an unknown id":       ids.From[ids.UserKind](ids.NewV7()),
	}
	due := clocktest.Now(t).Add(24 * time.Hour)
	mine := ids.From[ids.UserKind](e.Rep1)
	subject := "Chase the renewal"
	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &mine, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the task: %v", err)
	}
	for name, seat := range seats {
		t.Run(name, func(t *testing.T) {
			_, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &seat, Source: "manual",
			})
			assertSeatRefused(t, err, "assignee_id")
			_, err = e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(task.Id)),
				activities.UpdateActivityInput{AssigneeID: &seat})
			assertSeatRefused(t, err, "assignee_id")
			lead := "A lead for " + name
			_, _, err = e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &lead, Source: "manual", OwnerID: &seat})
			assertSeatRefused(t, err, "owner_id")
		})
	}
}

// assertSeatRefused holds a write to the one seat refusal, on the field the
// caller has to change.
func assertSeatRefused(t *testing.T, err error, field string) {
	t.Helper()
	refusal, ok := errors.AsType[*auth.AssigneeNotAllowedError](err)
	if !ok {
		t.Fatalf("got %v, want the seat refusal", err)
	}
	if got, code, _ := refusal.FieldFault(); got != field || code != "owner_not_assignable" {
		t.Errorf("the refusal names %s/%s, want %s/owner_not_assignable", got, code, field)
	}
}
