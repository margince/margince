// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Taking a task nobody holds, from the Unassigned Worklist: the row offers
// `take` only where the write it stands for would be admitted.

import (
	"errors"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// unheldTaskOn writes an overdue task on the contact through the real writer
// and leaves it to nobody.
func (r reminderEnv) unheldTaskOn(t *testing.T, subject string, contact ids.UUID) ids.ActivityID {
	t.Helper()
	due := r.now.AddDate(0, 0, -7)
	row, _, err := r.e.Activities.LogActivity(r.e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
	if _, err := r.e.Activities.UpdateActivity(r.e.Admin(), id, activities.UpdateActivityInput{Clear: []string{"assignee_id"}}); err != nil {
		t.Fatalf("leaving %q to nobody: %v", subject, err)
	}
	return id
}

func offersTake(item *crmcontracts.WorklistItem) bool {
	return slices.Contains(item.Actions, crmcontracts.WorklistItemActionsTake)
}

func (r reminderEnv) auditRowsOf(t *testing.T, task ids.ActivityID) int {
	t.Helper()
	return r.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, task.UUID)
}

func TestAColleagueTakesAnUnheldTaskOnARecordTheyMayChange(t *testing.T) {
	r := setupReminders(t)
	e := r.e
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	task := r.unheldTaskOn(t, "Send the renewal terms", contact)

	_, row := rowFor(r.worklist(r.rep2, t, "unassigned", ids.Nil), task)
	if row == nil {
		t.Fatal("the unheld task is missing from the Unassigned Worklist")
	}
	if !offersTake(row) || row.Version == nil {
		t.Fatalf("the row offers %v with version %v, want take and a version to pin it", row.Actions, row.Version)
	}

	before := r.auditRowsOf(t, task)
	store := e.Activities.WithClock(func() time.Time { return r.now })
	if _, err := store.UpdateActivity(r.rep2, task, activities.UpdateActivityInput{
		AssigneeID: new(ids.From[ids.UserKind](e.Rep2)), IfVersion: row.Version,
	}); err != nil {
		t.Fatalf("taking the task: %v", err)
	}

	if holder := e.WsScalar(t, `SELECT assignee_id::text FROM activity WHERE id = $1`, task.UUID); holder != e.Rep2.String() {
		t.Fatalf("the task is held by %q, want the colleague who took it %v", holder, e.Rep2)
	}
	if n := r.auditRowsOf(t, task) - before; n != 1 {
		t.Fatalf("taking the task wrote %d audit rows, want one", n)
	}
	if at, _ := rowFor(r.worklist(r.rep2, t, "unassigned", ids.Nil), task); at >= 0 {
		t.Error("the taken task still sits on the Unassigned Worklist")
	}
	if at, _ := rowFor(r.worklist(r.rep2, t, "mine", ids.Nil), task); at < 0 {
		t.Error("the taken task is missing from the taker's own Worklist")
	}
}

func TestAReaderWhoMayNotChangeTheRecordIsNotOfferedTakeAndIsRefused(t *testing.T) {
	r := setupReminders(t)
	e := r.e
	otherTeam := e.As(e.Rep3, []ids.UUID{e.Team2}, reminderRepPerms)
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	task := r.unheldTaskOn(t, "Send the renewal terms", contact)

	_, row := rowFor(r.worklist(otherTeam, t, "unassigned", ids.Nil), task)
	if row == nil {
		t.Fatal("the unheld task is missing from the Unassigned Worklist of a reader who may see it")
	}
	if offersTake(row) {
		t.Fatalf("a reader who may not change the task is offered take: %v", row.Actions)
	}

	before := r.auditRowsOf(t, task)
	_, err := e.Activities.UpdateActivity(otherTeam, task, activities.UpdateActivityInput{
		AssigneeID: new(ids.From[ids.UserKind](e.Rep3)), IfVersion: row.Version,
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("taking a task the reader may not change → %v, want the permission refusal (403)", err)
	}
	if holder := e.WsScalar(t, `SELECT coalesce(assignee_id::text, '') FROM activity WHERE id = $1`, task.UUID); holder != "" {
		t.Fatalf("the refused take left the task held by %q", holder)
	}
	if n := r.auditRowsOf(t, task) - before; n != 0 {
		t.Fatalf("the refused take wrote %d audit rows", n)
	}
}
