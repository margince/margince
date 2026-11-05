// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A meeting outcome row offers `cancel` only where the cancel is accepted. A
// live invitation refuses it, as does one whose guest left a shared meeting by
// erasure; a canceled one no longer holds the meeting.
func TestAMeetingOutcomeRowOffersCancelOnlyWhereTheWriteIsAccepted(t *testing.T) {
	e := integration.Setup(t)
	now := laneDay.Add(3 * time.Hour)
	// A signed-in member holds a full seat, and only a full seat is offered a write.
	reader := memberSeat(e, e.Rep1, principal.SeatFull)
	host := ids.From[ids.UserKind](e.Rep1)
	inviting := newInvitingStore(reader, t, e, now.AddDate(0, 0, -2))
	customer := func(name string) ids.UUID {
		created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: name})
		if err != nil {
			t.Fatalf("creating %q: %v", name, err)
		}
		return ids.UUID(created.Id)
	}
	booked := func(subject string) ids.UUID {
		at, status := now.AddDate(0, 0, -2), "booked"
		row, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{
			Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual", MeetingStatus: &status, HostUserID: &host,
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: customer("Customer for " + subject)}},
		})
		if err != nil {
			t.Fatalf("booking %q: %v", subject, err)
		}
		return ids.UUID(row.Id)
	}
	monday := func(hour int) time.Time { return time.Date(2026, time.March, 9, hour, 0, 0, 0, time.UTC) }
	confirmed := inviting.invite(t, customer("Confirmed guest"), monday(10))
	withdrawn := inviting.invite(t, customer("Withdrawn guest"), monday(12))
	inviting.cancel(t, withdrawn)
	reopen := "booked"
	if _, err := e.Activities.UpdateActivity(reader, ids.From[ids.ActivityKind](withdrawn), activities.UpdateActivityInput{MeetingStatus: &reopen}); err != nil {
		t.Fatalf("booking the withdrawn meeting again: %v", err)
	}
	erasedGuest := customer("Erased guest")
	shared := inviting.invite(t, erasedGuest, monday(14))
	if _, err := e.Activities.RelinkActivity(reader, ids.From[ids.ActivityKind](shared), activities.RelinkActivityInput{EntityType: "contact", EntityID: customer("Second attendee")}); err != nil {
		t.Fatalf("linking a second attendee, so the meeting outlives the erasure: %v", err)
	}
	if err := inviting.eraser.EraseContact(e.Admin(), erasedGuest, "subject request"); err != nil {
		t.Fatalf("erasing the guest: %v", err)
	}
	meetings := map[string]ids.UUID{
		"no invitation":                        booked("Booked in the app"),
		"live invitation":                      confirmed,
		"canceled invitation":                  withdrawn,
		"a guest erased from a shared meeting": shared,
	}
	offers := cancelOffers(reader, t, e, now)
	for name, meeting := range meetings {
		offered, read := offers[meeting]
		if !read {
			t.Fatalf("%s: no meeting_outcome row for the meeting", name)
		}
		stamped, err := e.Activities.GetActivity(reader, ids.From[ids.ActivityKind](meeting), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("%s: reading the meeting: %v", name, err)
		}
		canceled := "canceled"
		_, err = e.Activities.UpdateActivity(reader, ids.From[ids.ActivityKind](meeting), activities.UpdateActivityInput{MeetingStatus: &canceled})
		var refusal *activities.SchedulingArgumentError
		refused := errors.As(err, &refusal) && refusal.Code == "calendar_managed"
		if err != nil && !refused {
			t.Fatalf("%s: cancel: %v", name, err)
		}
		if offered == refused {
			t.Errorf("%s: the row offers cancel = %v while the write is refused = %v", name, offered, refused)
		}
		if managed := stamped.CalendarManaged != nil && *stamped.CalendarManaged; managed != refused {
			t.Errorf("%s: the read stamps calendar_managed = %v while the cancel is refused = %v", name, managed, refused)
		}
	}
	assertAHeldInvitedMeetingMovedAheadIsCalendarManaged(reader, t, e, confirmed)
}

// The invitation owns the start, so moving a held invited meeting ahead is
// refused for the invitation, not for an outcome the move would outrun.
func assertAHeldInvitedMeetingMovedAheadIsCalendarManaged(reader context.Context, t *testing.T, e *integration.Env, meeting ids.UUID) {
	t.Helper()
	id := ids.From[ids.ActivityKind](meeting)
	held := "held"
	if _, err := e.Activities.UpdateActivity(reader, id, activities.UpdateActivityInput{MeetingStatus: &held}); err != nil {
		t.Fatalf("recording the invited meeting as held: %v", err)
	}
	ahead := time.Date(2100, time.January, 4, 9, 0, 0, 0, time.UTC)
	_, err := e.Activities.UpdateActivity(reader, id, activities.UpdateActivityInput{OccurredAt: &ahead})
	var refusal *activities.SchedulingArgumentError
	if !errors.As(err, &refusal) || refusal.Code != "calendar_managed" {
		t.Errorf("moving the held invited meeting ahead answered %v, want the calendar_managed refusal", err)
	}
}

// A reader who may not change the meeting is offered no verb on its outcome
// row, and the row says why.
func TestAMeetingOutcomeRowForAReadSeatOffersNoVerb(t *testing.T) {
	e := integration.Setup(t)
	now := laneDay.Add(3 * time.Hour)
	guest, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Customer met"})
	if err != nil {
		t.Fatalf("creating the customer: %v", err)
	}
	host := ids.From[ids.UserKind](e.Rep1)
	at, status, subject := now.AddDate(0, 0, -2), "booked", "Met two days ago"
	meeting, _, err := e.Activities.LogActivity(memberSeat(e, e.Rep1, principal.SeatFull), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual", MeetingStatus: &status, HostUserID: &host,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(guest.Id)}},
	})
	if err != nil {
		t.Fatalf("booking the meeting: %v", err)
	}

	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	page, err := feed.Worklist(memberSeat(e, e.Rep1, principal.SeatRead), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the worklist: %v", err)
	}
	i := slices.IndexFunc(page.Queue, func(row crmcontracts.WorklistItem) bool {
		return row.Source == crmcontracts.WorklistItemSourceMeetingOutcome && row.Id == meeting.Id.String()
	})
	if i < 0 {
		t.Fatal("the read seat has no meeting_outcome row for the meeting")
	}
	row := page.Queue[i]
	if len(row.Actions) != 0 {
		t.Errorf("the read seat is offered %v, want no verb", row.Actions)
	}
	if !slices.ContainsFunc(row.Because, func(why crmcontracts.WorklistReason) bool { return why.Kind == "read_only" }) {
		t.Errorf("the row gives %v, want a read_only reason", row.Because)
	}
}

// A live invitation holds its meeting, so undoing the meeting's create is
// refused for it. A canceled one holds nothing.
func TestOnlyALiveInvitationRefusesTheMeetingCreateUndo(t *testing.T) {
	e := integration.Setup(t)
	start := time.Date(2026, time.March, 9, 10, 0, 0, 0, time.UTC)
	inviting := newInvitingStore(memberSeat(e, e.Rep1, principal.SeatFull), t, e, start.AddDate(0, 0, -2))
	const heldByInvitation = "a meeting with a calendar invitation"
	live := inviting.invite(t, e.SeedContact(t, "Invited Ines", nil), start)
	if answer := advisoryAnswer(e.Admin(), t, e, entityTypeActivity, live, latestAuditRowID(t, e, entityTypeActivity, live, actionCreate)); answer.Detail != heldByInvitation {
		t.Fatalf("the live invitation's meeting create was answered %+v, want refused for %q", answer, heldByInvitation)
	}

	withdrawn := inviting.invite(t, e.SeedContact(t, "Withdrawn Wim", nil), start.Add(2*time.Hour))
	inviting.cancel(t, withdrawn)
	if answer := advisoryAnswer(e.Admin(), t, e, entityTypeActivity, withdrawn, latestAuditRowID(t, e, entityTypeActivity, withdrawn, actionCreate)); answer.Detail == heldByInvitation {
		t.Errorf("the canceled invitation's meeting create was refused for %q, which it no longer holds", heldByInvitation)
	}
}

// memberSeat signs user in as a member holding seat, with every grant.
func memberSeat(e *integration.Env, user ids.UUID, seat principal.SeatType) context.Context {
	return principal.WithActor(e.As(user, nil, integration.AdminPerms), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: integration.AdminPerms, SeatType: seat,
	})
}

// invitingStore is the scheduling engine as the host, its clock at the moment
// the invitations went out, and the eraser that shares its vault.
type invitingStore struct {
	ctx    context.Context
	store  *activities.Store
	eraser *privacy.Eraser
}

func newInvitingStore(ctx context.Context, t *testing.T, e *integration.Env, sentAt time.Time) invitingStore {
	t.Helper()
	vault := keyvault.NewMemory()
	store := e.Activities.WithClock(func() time.Time { return sentAt }).WithPublicBaseURL("https://crm.example.test").
		WithMeetingVault(vault).WithSchedulingCalendar(&erasingCalendar{erase: func() error { return nil }}).
		WithWorkingHours(func(context.Context, ids.UserID) (activities.WorkingHours, error) {
			return activities.WorkingHours{StartMinute: 540, EndMinute: 1020, Days: []int{1, 2, 3, 4, 5}, Location: time.UTC}, nil
		})
	if _, err := store.SaveSchedulingProfile(ctx, crmcontracts.SchedulingProfile{Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Discovery"}); err != nil {
		t.Fatalf("saving the scheduling profile: %v", err)
	}
	return invitingStore{ctx: ctx, store: store, eraser: privacy.NewEraser(e.DB()).WithPayloadVault(controllerPayloads{v: vault})}
}

// invite books guest at start through the invitation engine and delivers it.
func (s invitingStore) invite(t *testing.T, guest ids.UUID, start time.Time) ids.UUID {
	t.Helper()
	meeting, err := s.store.CreateInvitation(s.ctx, crmcontracts.MeetingInvitationRequest{
		ContactId: crmcontracts.Id(guest), AttendeeEmail: openapi_types.Email(fmt.Sprintf("guest-%d@example.test", start.Hour())), Subject: "Invited for " + start.Format(time.Kitchen),
		Start: start, End: start.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("inviting for %v: %v", start, err)
	}
	s.deliver(t)
	return ids.UUID(meeting.Id)
}

// eraseGuest erases a meeting's sole guest, which withdraws its invitation.
func (s invitingStore) eraseGuest(t *testing.T, e *integration.Env, guest, meeting ids.UUID) {
	t.Helper()
	if err := s.eraser.EraseContact(e.Admin(), guest, "subject request"); err != nil {
		t.Fatalf("erasing the guest: %v", err)
	}
	if got := e.WsScalar(t, `SELECT status FROM meeting_invitation WHERE activity_id = $1`, meeting); got != "erased" {
		t.Fatalf("the invitation reads %q after its guest was erased, want erased", got)
	}
}

// cancel withdraws the invitation and delivers the cancellation.
func (s invitingStore) cancel(t *testing.T, meeting ids.UUID) {
	t.Helper()
	current, err := s.store.Invitation(s.ctx, meeting)
	if err != nil {
		t.Fatalf("reading the invitation: %v", err)
	}
	if _, err := s.store.ChangeInvitation(s.ctx, meeting, crmcontracts.MeetingInvitationChange{Action: "cancel", Version: current.Version}); err != nil {
		t.Fatalf("canceling the invitation: %v", err)
	}
	s.deliver(t)
}

func (s invitingStore) deliver(t *testing.T) {
	t.Helper()
	if err := s.store.DeliverInvitations(s.ctx); err != nil {
		t.Fatalf("delivering invitations: %v", err)
	}
}

// cancelOffers reads the reader's meeting_outcome rows and whether each offers cancel.
func cancelOffers(reader context.Context, t *testing.T, e *integration.Env, now time.Time) map[ids.UUID]bool {
	t.Helper()
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	page, err := feed.Worklist(reader, "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the worklist: %v", err)
	}
	out := map[ids.UUID]bool{}
	for _, row := range page.Queue {
		if row.Source != crmcontracts.WorklistItemSourceMeetingOutcome {
			continue
		}
		id, err := ids.Parse(row.Id)
		if err != nil {
			t.Fatalf("row id %q: %v", row.Id, err)
		}
		out[id] = slices.Contains(row.Actions, crmcontracts.WorklistItemActionsCancel)
	}
	return out
}
