// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type invitationFixture struct {
	env      *sendEnv
	ctx      context.Context
	store    *Store
	calendar *invitationCalendar
	now      time.Time
	request  crmcontracts.MeetingInvitationRequest
}

func newInvitationFixture(t *testing.T) *invitationFixture {
	t.Helper()
	e := setupSend(t)
	f := &invitationFixture{env: e, ctx: e.as(principal.RowScopeAll), calendar: &invitationCalendar{}, now: monday(6)}
	f.store = e.store(nil).WithClock(func() time.Time { return f.now }).WithWorkingHours(func(context.Context, ids.UserID) (WorkingHours, error) { return fallbackWorkingHours(), nil }).WithSchedulingCalendar(f.calendar).WithMeetingVault(keyvault.NewMemory()).WithPublicBaseURL("https://crm.example.com")
	profile := defaultSchedulingProfile()
	profile.Provider = "gcal"
	profile.BufferMinutes = 0
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	contact := ids.NewV7()
	args := schedulingArgs{}
	if _, err := e.owner.Exec(f.ctx, `INSERT INTO contact(id,full_name,source,captured_by)VALUES(`+args.add(contact)+`,'Guest','manual','human:test')`, args...); err != nil {
		t.Fatal(err)
	}
	f.request = crmcontracts.MeetingInvitationRequest{ContactId: crmcontracts.Id(contact), AttendeeEmail: "guest@example.test", Subject: "Project meeting", Start: monday(10), End: monday(11)}
	return f
}

func (f *invitationFixture) book(t *testing.T) crmcontracts.MeetingInvitation {
	t.Helper()
	meeting, err := f.store.CreateInvitation(f.ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	meeting, err = f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil {
		t.Fatal(err)
	}
	if meeting.Status != "confirmed" {
		t.Fatalf("not confirmed: %+v", meeting)
	}
	return meeting
}

func TestRescheduleKeepsTheOriginalReservationUntilAcknowledged(t *testing.T) {
	f := newInvitationFixture(t)
	meeting := f.book(t)
	start, end := monday(13), monday(14)
	_, err := f.store.ChangeInvitation(f.ctx, ids.UUID(meeting.Id), crmcontracts.MeetingInvitationChange{Action: "reschedule", Version: meeting.Version, Start: &start, End: &end})
	if err != nil {
		t.Fatal(err)
	}
	for _, interval := range []slot{{Start: f.request.Start, End: f.request.End}, {Start: start, End: end}} {
		other := f.request
		other.Start, other.End = interval.Start, interval.End
		_, err := f.store.CreateInvitation(f.ctx, other)
		if _, ok := errors.AsType[*SlotTakenError](err); !ok {
			t.Fatalf("lost old or proposed reservation: %v", err)
		}
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CreateInvitation(f.ctx, f.request); err != nil {
		t.Fatalf("old reservation not released: %v", err)
	}
	var occurred time.Time
	if err := f.env.owner.QueryRow(f.ctx, `SELECT occurred_at FROM activity WHERE id=$1`, meeting.Id).Scan(&occurred); err != nil {
		t.Fatal(err)
	}
	if !occurred.Equal(start) {
		t.Fatalf("activity did not move: %v", occurred)
	}
}

// A held meeting rescheduled has not happened at its new slot. So settling the
// move books it again, even when the receipt lands after the new slot began.
func TestAHeldMeetingRescheduledSettlesBooked(t *testing.T) {
	for name, settledAt := range map[string]time.Time{
		"settled before the new slot":      monday(12),
		"settled after the new slot began": monday(17),
	} {
		t.Run(name, func(t *testing.T) {
			f := newInvitationFixture(t)
			meeting := f.book(t)
			id := ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
			f.now = monday(12)
			held := string(crmcontracts.ActivityMeetingStatusHeld)
			if _, err := f.store.UpdateActivity(f.ctx, id, UpdateActivityInput{MeetingStatus: &held}); err != nil {
				t.Fatalf("recording the meeting as held: %v", err)
			}
			start, end := monday(15), monday(16)
			if _, err := f.store.ChangeInvitation(f.ctx, ids.UUID(meeting.Id), crmcontracts.MeetingInvitationChange{Action: "reschedule", Version: meeting.Version, Start: &start, End: &end}); err != nil {
				t.Fatalf("rescheduling: %v", err)
			}
			f.calendar.saved = func() { f.now = settledAt }
			if err := f.store.DeliverInvitations(f.ctx); err != nil {
				t.Fatalf("settling the reschedule: %v", err)
			}
			var status string
			if err := f.env.owner.QueryRow(f.ctx, `SELECT meeting_status FROM activity WHERE id=$1`, meeting.Id).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "booked" {
				t.Errorf("the rescheduled meeting reads %q, want booked", status)
			}
		})
	}
}

// Only a Margince reschedule takes an outcome away. A provider change that
// leaves the start where it was keeps it, on a meeting rescheduled before too.
func TestAHeldRescheduledMeetingKeepsItsOutcomeWhenTheProviderMovesOnlyTheEnd(t *testing.T) {
	f := newInvitationFixture(t)
	meeting := f.book(t)
	id := ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	start, end := monday(13), monday(14)
	if _, err := f.store.ChangeInvitation(f.ctx, ids.UUID(meeting.Id), crmcontracts.MeetingInvitationChange{Action: "reschedule", Version: meeting.Version, Start: &start, End: &end}); err != nil {
		t.Fatalf("rescheduling: %v", err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatalf("settling the reschedule: %v", err)
	}
	f.now = monday(15)
	held := string(crmcontracts.ActivityMeetingStatusHeld)
	if _, err := f.store.UpdateActivity(f.ctx, id, UpdateActivityInput{MeetingStatus: &held}); err != nil {
		t.Fatalf("recording the moved meeting as held: %v", err)
	}
	transitions := func() (count int) {
		if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM activity_meeting_history WHERE activity_id=$1`, meeting.Id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	before := transitions()
	var row invitationRow
	if err := f.store.tx(f.ctx, func(tx pgx.Tx) error {
		var err error
		row, err = readInvitation(f.ctx, tx, ids.UUID(meeting.Id), false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.reconcileInvitation(f.ctx, row, connector.CalendarState{Start: start, End: monday(15)}); err != nil {
		t.Fatalf("importing the provider's new end: %v", err)
	}
	var status string
	var seconds int
	if err := f.env.owner.QueryRow(f.ctx, `SELECT meeting_status, duration_seconds FROM activity WHERE id=$1`, meeting.Id).Scan(&status, &seconds); err != nil {
		t.Fatal(err)
	}
	if seconds != 2*60*60 {
		t.Fatalf("the provider's new end was not imported: %d seconds", seconds)
	}
	if status != held {
		t.Errorf("the meeting reads %q after an end-only provider change, want held", status)
	}
	if after := transitions(); after != before {
		t.Errorf("an end-only provider change wrote %d status transitions, want none", after-before)
	}
}

// A reschedule start carries nanoseconds the stored start drops. Settling that
// reschedule again, as a retry does, still finds the outcome at its own start.
func TestSettlingASubMicrosecondRescheduleAgainKeepsItsOutcome(t *testing.T) {
	f := newInvitationFixture(t)
	meeting := f.book(t)
	id := ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	start, end := monday(13).Add(789*time.Nanosecond), monday(14)
	if _, err := f.store.ChangeInvitation(f.ctx, ids.UUID(meeting.Id), crmcontracts.MeetingInvitationChange{Action: "reschedule", Version: meeting.Version, Start: &start, End: &end}); err != nil {
		t.Fatalf("rescheduling: %v", err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatalf("settling the reschedule: %v", err)
	}
	f.now = monday(15)
	held := string(crmcontracts.ActivityMeetingStatusHeld)
	if _, err := f.store.UpdateActivity(f.ctx, id, UpdateActivityInput{MeetingStatus: &held}); err != nil {
		t.Fatalf("recording the moved meeting as held: %v", err)
	}
	if err := f.store.tx(f.ctx, func(tx pgx.Tx) error {
		row, err := readInvitation(f.ctx, tx, ids.UUID(meeting.Id), false)
		if err != nil {
			return err
		}
		return f.store.settleInvitation(f.ctx, tx, row, true)
	}); err != nil {
		t.Fatalf("settling the reschedule again: %v", err)
	}
	var status string
	if err := f.env.owner.QueryRow(f.ctx, `SELECT meeting_status FROM activity WHERE id=$1`, meeting.Id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != held {
		t.Errorf("the meeting reads %q after its reschedule settled again, want held", status)
	}
}

func TestAProposalIsConsumedInTheInvitationTransaction(t *testing.T) {
	f := newInvitationFixture(t)
	link, err := f.store.CreateProposal(f.ctx, crmcontracts.MeetingProposalRequest{ContactId: f.request.ContactId, AttendeeEmail: f.request.AttendeeEmail, Subject: f.request.Subject, DurationMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Split(link.URL, "proposal-")[1]
	id, host, err := f.store.ResolveProposalToken(f.ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	booked, err := f.store.reserveAndQueueInvitation(f.ctx, host, f.request, invitationIntent{ProposalID: id})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := f.store.readProposal(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := f.store.recoverProposalInvitation(f.ctx, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Id != booked.Id || recovered.ManagementToken == nil || *recovered.ManagementToken != *booked.ManagementToken || recovered.CalendarUrl != nil {
		t.Fatalf("cannot recover the same guest booking: %+v", recovered)
	}
	later := f.request
	later.Start, later.End = monday(13), monday(14)
	if _, err := f.store.reserveAndQueueInvitation(f.ctx, host, later, invitationIntent{ProposalID: id}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("reused proposal: %v", err)
	}
	var count int
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM meeting_invitation`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("one-use link wrote %d meetings", count)
	}
}

func TestProviderEchoResolvesBeforeTheDeliveryReceipt(t *testing.T) {
	f := newInvitationFixture(t)
	meeting, err := f.store.CreateInvitation(f.ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	err = f.store.tx(f.ctx, func(tx pgx.Tx) error {
		id, found, err := ResolveCalendarInvitation(f.ctx, tx, "gcal", strings.ReplaceAll(ids.UUID(meeting.Id).String(), "-", ""), ids.UUID(meeting.Id).String())
		if err != nil {
			return err
		}
		if !found || id.UUID != ids.UUID(meeting.Id) {
			t.Fatalf("echo did not resolve: %v %v", id, found)
		}
		actor, _ := principal.Actor(f.ctx)
		actor.UserID = f.env.other
		_, found, err = ResolveCalendarInvitation(principal.WithActor(f.ctx, actor), tx, "gcal", "event", ids.UUID(meeting.Id).String())
		if found {
			t.Fatal("another host joined the organizer's pending invitation")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProviderRescheduleStillBlocksItsActualTime(t *testing.T) {
	f := newInvitationFixture(t)
	meeting := f.book(t)
	var row invitationRow
	if err := f.store.tx(f.ctx, func(tx pgx.Tx) error {
		var err error
		row, err = readInvitation(f.ctx, tx, ids.UUID(meeting.Id), false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state := connector.CalendarState{Start: monday(13), End: monday(15)}
	if err := f.store.reconcileInvitation(f.ctx, row, state); err != nil {
		t.Fatal(err)
	}
	request := f.request
	request.Start, request.End = monday(14), monday(15)
	_, err := f.store.CreateInvitation(f.ctx, request)
	if _, ok := errors.AsType[*SlotTakenError](err); !ok {
		t.Fatalf("provider change left its second hour available: %v", err)
	}
}

func TestContactActivityCarriesTheRealInvitationDeliveryStatus(t *testing.T) {
	f := newInvitationFixture(t)
	meeting, err := f.store.CreateInvitation(f.ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	activity, err := f.store.GetActivity(f.ctx, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if activity.InvitationStatus == nil || *activity.InvitationStatus != "pending" {
		t.Fatalf("pending delivery missing from timeline: %+v", activity.InvitationStatus)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	activity, err = f.store.GetActivity(f.ctx, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if activity.InvitationStatus == nil || *activity.InvitationStatus != "confirmed" {
		t.Fatalf("confirmed delivery missing from timeline: %+v", activity.InvitationStatus)
	}
}

func TestAnUncertainCancellationKeepsTheTimeReserved(t *testing.T) {
	f := newInvitationFixture(t)
	f.calendar.fail = true
	f.calendar.lookupErr = connector.ErrUnreachable
	meeting, err := f.store.CreateInvitation(f.ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := f.store.DeliverInvitations(f.ctx); err != nil {
			t.Fatal(err)
		}
		f.now = f.now.Add(10 * time.Minute)
	}
	meeting, err = f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil {
		t.Fatal(err)
	}
	if meeting.Status != invitationNeedsAttention {
		t.Fatalf("unexpected delivery state: %s", meeting.Status)
	}
	if _, err := f.store.ChangeInvitation(f.ctx, ids.UUID(meeting.Id), crmcontracts.MeetingInvitationChange{Action: "cancel", Version: meeting.Version}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	meeting, err = f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil {
		t.Fatal(err)
	}
	if meeting.Status == invitationCanceled {
		t.Fatal("an absent receipt was treated as proof of cancellation")
	}
	_, err = f.store.CreateInvitation(f.ctx, f.request)
	if _, ok := errors.AsType[*SlotTakenError](err); !ok {
		t.Fatalf("uncertain cancellation released its reservation: %v", err)
	}
	if f.calendar.canceled != 0 {
		t.Fatal("canceled an event without recovering its identity")
	}
}
