// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package tasktaken_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/tasktaken"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

var colleaguePerms = principal.Permissions{
	RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
	Objects: map[string]principal.ObjectGrant{
		"activity": {Read: true, Update: true},
		"contact":  {Read: true},
	},
}

// seatOf is user as a full seat, the kind that may hold work.
func seatOf(e *integration.Env, user ids.UUID) context.Context {
	return integration.OnSeat(e.As(user, nil, colleaguePerms), principal.SeatFull)
}

// unassignedTaskOn files an open task under contact and clears the assignee a
// task filed by hand would get, as automatic work leaves it.
func unassignedTaskOn(t *testing.T, e *integration.Env, contact ids.UUID) ids.ActivityID {
	t.Helper()
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: new("Send the price list"), Source: provenance.RecordSourceManual,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("filing the task: %v", err)
	}
	e.WsExec(t, `UPDATE activity SET assignee_id = NULL WHERE id = $1`, row.Id)
	return ids.From[ids.ActivityKind](ids.UUID(row.Id))
}

func TestTakingATaskTellsTheOwnerOfItsRecordOnce(t *testing.T) {
	e := integration.Setup(t)
	e.Activities.WithTaskTaken(tasktaken.Hook())
	contact := e.SeedContact(t, "Ada Example", &e.Rep1)
	task := unassignedTaskOn(t, e, contact)
	version := int64(e.WsCount(t, `SELECT version FROM activity WHERE id = $1`, task.UUID))

	if _, err := e.Activities.TakeTask(seatOf(e, e.Rep2), task, version); err != nil {
		t.Fatalf("taking the task: %v", err)
	}

	if got := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'task_taken' AND recipient_user_id = $1
		AND target_type = 'contact' AND target_id = $2`, e.Rep1, contact); got != 1 {
		t.Errorf("%d notices reached the contact's owner, want 1", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'task_taken' AND recipient_user_id = $1`, e.Rep2); got != 0 {
		t.Errorf("the taker was told about their own take %d times", got)
	}
}

func TestTheOwnerTakingTheirOwnRecordsTaskTellsNobody(t *testing.T) {
	e := integration.Setup(t)
	e.Activities.WithTaskTaken(tasktaken.Hook())
	contact := e.SeedContact(t, "Ada Example", &e.Rep1)
	task := unassignedTaskOn(t, e, contact)
	version := int64(e.WsCount(t, `SELECT version FROM activity WHERE id = $1`, task.UUID))

	if _, err := e.Activities.TakeTask(seatOf(e, e.Rep1), task, version); err != nil {
		t.Fatalf("taking the task: %v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'task_taken'`); got != 0 {
		t.Errorf("%d notices for an owner who took their own task, want none", got)
	}
}

func TestATaskOnARecordWithNoOwnerTellsNobody(t *testing.T) {
	e := integration.Setup(t)
	e.Activities.WithTaskTaken(tasktaken.Hook())
	contact := e.SeedContact(t, "Ada Example", nil)
	e.WsExec(t, `UPDATE contact SET owner_id = NULL WHERE id = $1`, contact)
	task := unassignedTaskOn(t, e, contact)
	version := int64(e.WsCount(t, `SELECT version FROM activity WHERE id = $1`, task.UUID))

	if _, err := e.Activities.TakeTask(seatOf(e, e.Rep2), task, version); err != nil {
		t.Fatalf("taking the task: %v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'task_taken'`); got != 0 {
		t.Errorf("%d notices for a record nobody owns, want none", got)
	}
}

func takeAsRep2(t *testing.T, e *integration.Env, task ids.ActivityID) {
	t.Helper()
	version := int64(e.WsCount(t, `SELECT version FROM activity WHERE id = $1`, task.UUID))
	if _, err := e.Activities.TakeTask(seatOf(e, e.Rep2), task, version); err != nil {
		t.Fatalf("taking the task: %v", err)
	}
}

func linkTask(t *testing.T, e *integration.Env, task ids.ActivityID, contact ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id) VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), task.UUID, contact)
}

func noticesTo(t *testing.T, e *integration.Env, user ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'task_taken' AND recipient_user_id = $1`, user)
}

// The taker sees the task through one record only. The owner of the record they
// cannot read is still told, as is the owner of the one they can.
func TestEveryLinkedRecordsOwnerIsToldNotOnlyThoseTheTakerCanRead(t *testing.T) {
	e := integration.Setup(t)
	e.Activities.WithTaskTaken(tasktaken.Hook())
	seen := e.SeedContact(t, "Ada Example", &e.Rep1)
	hidden := e.SeedContact(t, "Bo Private", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", hidden, e.Rep3)
	task := unassignedTaskOn(t, e, seen)
	linkTask(t, e, task, hidden)

	takeAsRep2(t, e, task)

	for name, owner := range map[string]ids.UUID{"the visible record's owner": e.Rep1, "the private record's owner": e.Rep3} {
		if got := noticesTo(t, e, owner); got != 1 {
			t.Errorf("%s got %d notices, want 1", name, got)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'task_taken' AND (subject LIKE '%price list%' OR body LIKE '%price list%')`); got != 0 {
		t.Errorf("%d stored notices carry the task's text", got)
	}
}

func TestAnOwnerOfTwoLinkedRecordsIsToldOnce(t *testing.T) {
	e := integration.Setup(t)
	e.Activities.WithTaskTaken(tasktaken.Hook())
	first := e.SeedContact(t, "Ada Example", &e.Rep1)
	second := e.SeedContact(t, "Bo Example", &e.Rep1)
	task := unassignedTaskOn(t, e, first)
	linkTask(t, e, task, second)

	takeAsRep2(t, e, task)

	if got := noticesTo(t, e, e.Rep1); got != 1 {
		t.Errorf("the shared owner got %d notices, want 1", got)
	}
}

func TestAnOwnerWhoLeftIsToldNothing(t *testing.T) {
	e := integration.Setup(t)
	e.Activities.WithTaskTaken(tasktaken.Hook())
	left := e.SeedContact(t, "Ada Example", &e.Rep3)
	task := unassignedTaskOn(t, e, left)
	e.WsExec(t, `UPDATE app_user SET status = 'deactivated', removed_at = now() WHERE id = $1`, e.Rep3)

	takeAsRep2(t, e, task)

	if got := noticesTo(t, e, e.Rep3); got != 0 {
		t.Errorf("a departed owner got %d notices, want none", got)
	}
}

func TestAnOwnerOutsideASelectedAudienceLearnsNothing(t *testing.T) {
	e := integration.Setup(t)
	e.Activities.WithTaskTaken(tasktaken.Hook())
	contact := e.SeedContact(t, "Ada Example", &e.Rep1)
	task := unassignedTaskOn(t, e, contact)
	e.WsExec(t, `UPDATE activity SET audience = 'selected' WHERE id = $1`, task.UUID)
	e.WsExec(t, `INSERT INTO activity_audience_member (activity_id, subject_type, subject_id, created_by) VALUES ($1, 'user', $2, 'test')`, task.UUID, e.Rep2)

	takeAsRep2(t, e, task)

	if got := noticesTo(t, e, e.Rep1); got != 0 {
		t.Errorf("an owner outside the task's audience got %d notices, want none", got)
	}
}
