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
