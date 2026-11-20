// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (f *invitationFixture) handTask(t *testing.T, subject, entityType string, record ids.UUID) ids.UUID {
	t.Helper()
	task, _, err := f.store.LogActivity(f.ctx, LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual", AssigneeID: new(ids.From[ids.UserKind](f.env.rep)),
		Links: []ActivityLinkInput{{EntityType: entityType, EntityID: record}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(task.Id)
}

func (f *invitationFixture) colleagueTask(t *testing.T, record ids.UUID) ids.UUID {
	t.Helper()
	colleague := ids.NewV7()
	if _, err := f.env.owner.Exec(f.ctx, `INSERT INTO app_user (id, email, display_name) VALUES ($1,$2,'Colleague')`, colleague, "colleague-"+colleague.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	task, _, err := f.store.LogActivity(f.ctx, LogActivityInput{
		Kind: "task", Subject: new("Colleague's task"), Source: "manual", AssigneeID: new(ids.From[ids.UserKind](colleague)),
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: record}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(task.Id)
}

func (f *invitationFixture) taskIsDone(t *testing.T, task ids.UUID) bool {
	t.Helper()
	var done bool
	if err := f.env.owner.QueryRow(f.ctx, `SELECT is_done FROM activity WHERE id=$1`, task).Scan(&done); err != nil {
		t.Fatal(err)
	}
	return done
}

func (f *invitationFixture) seedLead(t *testing.T, email string) ids.UUID {
	t.Helper()
	lead := ids.NewV7()
	if _, err := f.env.owner.Exec(f.ctx, `INSERT INTO lead (id, full_name, email, status, source, captured_by, owner_id)
 VALUES ($1,'Lead A',$3,'new','manual','human:test',$2)`, lead, f.env.rep, email); err != nil {
		t.Fatal(err)
	}
	return lead
}

func (f *invitationFixture) bookThroughThePage(t *testing.T, intent invitationIntent) crmcontracts.MeetingInvitation {
	t.Helper()
	host, err := schedulingHost(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	door := principal.SystemActing(f.ctx, "system:public_booking")
	intent.DescriptionFromBooker = true
	meeting, err := f.store.reserveAndQueueInvitation(door, host, f.request, intent)
	if err != nil {
		t.Fatal(err)
	}
	return meeting
}

func (f *invitationFixture) systemCtx() context.Context {
	return principal.SystemActing(f.ctx, "system:test")
}

func TestABookingThroughThePageClosesTheOpenTasksOnTheBookedLead(t *testing.T) {
	f := newInvitationFixture(t)
	lead := f.seedLead(t, "guest@example.test")
	other := f.seedLead(t, "other@example.test")
	mine := f.handTask(t, "Send Lead A the booking link again", "lead", lead)
	elsewhere := f.handTask(t, "Another lead", "lead", other)
	finished := f.handTask(t, "Already finished", "lead", lead)
	if _, err := f.store.UpdateActivity(f.ctx, ids.From[ids.ActivityKind](finished), UpdateActivityInput{IsDone: new(true)}); err != nil {
		t.Fatal(err)
	}
	meeting := f.bookThroughThePage(t, invitationIntent{Lead: ids.From[ids.LeadKind](lead)})
	after := f.handTask(t, "Prepare the demo", "lead", lead)

	if !f.taskIsDone(t, mine) {
		t.Fatal("the user's own task stayed open after the lead booked")
	}
	if f.taskIsDone(t, elsewhere) || f.taskIsDone(t, after) {
		t.Fatal("a task on another record, or created after the booking, was closed")
	}
	actor, _ := principal.Actor(f.ctx)
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true}
	receipts, err := f.store.RecentTaskCompletions(principal.WithActor(f.ctx, actor), clocktest.Now(t).Add(-time.Hour), 10)
	if err != nil || len(receipts) != 1 || receipts[0].ID != mine || !receipts[0].Booking || receipts[0].Contact != "Lead A" || receipts[0].EmailID != ids.UUID(meeting.Id) {
		t.Fatalf("receipts=%+v, %v", receipts, err)
	}

	if _, err := f.store.UpdateActivity(f.ctx, ids.From[ids.ActivityKind](mine), UpdateActivityInput{IsDone: new(false)}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.tx(f.systemCtx(), func(tx pgx.Tx) error {
		n, err := f.store.CloseTasksAnsweredByBookingTx(f.systemCtx(), tx, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), ids.From[ids.UserKind](f.env.rep), []ActivityLinkInput{{EntityType: "lead", EntityID: lead}})
		if n != 0 {
			t.Fatalf("a second pass over the same booking closed %d tasks", n)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if f.taskIsDone(t, mine) || f.taskIsDone(t, after) {
		t.Fatal("the booking closed a task again after the user's Undo, or one created after it")
	}
}

func TestABookingThroughThePageClosesTheOpenTasksOnTheBookedContactOnly(t *testing.T) {
	f := newInvitationFixture(t)
	contact := ids.UUID(f.request.ContactId)
	mine := f.handTask(t, "Call the guest", "contact", contact)
	colleagues := f.colleagueTask(t, contact)
	minted := ids.NewV7()
	if _, err := f.env.owner.Exec(f.ctx, `INSERT INTO activity (id, kind, subject, occurred_at, due_at, source, captured_by, assignee_id)
 VALUES ($1,'task','Review qualification',now(),now()+interval '1 day','system','system',$2)`, minted, f.env.rep); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.owner.Exec(f.ctx, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1,'contact',$2)`, minted, contact); err != nil {
		t.Fatal(err)
	}
	f.bookThroughThePage(t, invitationIntent{})
	if f.taskIsDone(t, minted) {
		t.Fatal("a task Margince made was closed by the user-made-task rule")
	}
	if !f.taskIsDone(t, mine) {
		t.Fatal("the user's own task on the booked contact stayed open")
	}
	if f.taskIsDone(t, colleagues) {
		t.Fatal("an anonymous booking closed another user's task")
	}
}

func TestABookingMadeByTheUserClosesNoTask(t *testing.T) {
	f := newInvitationFixture(t)
	mine := f.handTask(t, "Call the guest", "contact", ids.UUID(f.request.ContactId))
	if _, err := f.store.CreateInvitation(f.ctx, f.request); err != nil {
		t.Fatal(err)
	}
	if f.taskIsDone(t, mine) {
		t.Fatal("a meeting the user booked closed a task")
	}
}

func TestACanceledBookingReopensTheTasksItClosedUnlessTheUserTouchedThem(t *testing.T) {
	f := newInvitationFixture(t)
	contact := ids.UUID(f.request.ContactId)
	untouched := f.handTask(t, "Call the guest", "contact", contact)
	touched := f.handTask(t, "Send the deck", "contact", contact)
	meeting := f.bookThroughThePage(t, invitationIntent{})
	for _, id := range []ids.UUID{untouched, touched} {
		if !f.taskIsDone(t, id) {
			t.Fatal("the booking left a task open")
		}
	}
	for _, done := range []bool{false, true} {
		if _, err := f.store.UpdateActivity(f.ctx, ids.From[ids.ActivityKind](touched), UpdateActivityInput{IsDone: &done}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := f.store.ReopenTasksAnsweredByMeeting(f.systemCtx(), ids.From[ids.ActivityKind](ids.UUID(meeting.Id))); err != nil || n != 0 {
		t.Fatalf("a booking still standing reopened %d tasks: %v", n, err)
	}
	if _, err := f.store.ChangeInvitation(f.ctx, ids.UUID(meeting.Id), crmcontracts.MeetingInvitationChange{Action: "cancel", Version: meeting.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.owner.Exec(f.ctx, `UPDATE activity SET meeting_status='canceled' WHERE id=$1`, ids.UUID(meeting.Id)); err != nil {
		t.Fatal(err)
	}
	n, err := f.store.ReopenTasksAnsweredByMeeting(f.systemCtx(), ids.From[ids.ActivityKind](ids.UUID(meeting.Id)))
	if err != nil || n != 1 {
		t.Fatalf("reopened %d tasks: %v", n, err)
	}
	if f.taskIsDone(t, untouched) || !f.taskIsDone(t, touched) {
		t.Fatal("the cancel must reopen the untouched task and leave the one the user finished")
	}
}

type readOnlyHost struct{ *invitationCalendar }

func (readOnlyHost) HostMayWrite(context.Context, pgx.Tx, ids.UserID, string, ids.UUID) (bool, error) {
	return false, nil
}

func TestABookingOnARecordTheHostCannotWriteClosesNothing(t *testing.T) {
	f := newInvitationFixture(t)
	mine := f.handTask(t, "Call the guest", "contact", ids.UUID(f.request.ContactId))
	f.store = f.store.WithSchedulingCalendar(readOnlyHost{f.calendar})
	f.bookThroughThePage(t, invitationIntent{})
	if f.taskIsDone(t, mine) {
		t.Fatal("a booking closed the host's task on a record the host may not write")
	}
}

func TestACanceledBookingLeavesATaskTheHostWroteToSince(t *testing.T) {
	f := newInvitationFixture(t)
	mine := f.handTask(t, "Call the guest", "contact", ids.UUID(f.request.ContactId))
	meeting := f.bookThroughThePage(t, invitationIntent{})
	if _, err := f.store.UpdateActivity(f.ctx, ids.From[ids.ActivityKind](mine), UpdateActivityInput{Subject: new("Call the guest tomorrow")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.owner.Exec(f.ctx, `UPDATE activity SET meeting_status='canceled' WHERE id=$1`, ids.UUID(meeting.Id)); err != nil {
		t.Fatal(err)
	}
	if n, err := f.store.ReopenTasksAnsweredByMeeting(f.systemCtx(), ids.From[ids.ActivityKind](ids.UUID(meeting.Id))); err != nil || n != 0 {
		t.Fatalf("reopened %d tasks the host edited: %v", n, err)
	}
	if !f.taskIsDone(t, mine) {
		t.Fatal("the cancel reopened a task the host had edited")
	}
}
