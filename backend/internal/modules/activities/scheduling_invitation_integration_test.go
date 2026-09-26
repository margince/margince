// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type invitationCalendar struct {
	requests   []connector.CalendarAppointment
	fail       bool
	busy       []connector.CalendarInterval
	canceled   int
	lookupErr  error
	inspectErr error
}

func (c *invitationCalendar) Check(context.Context, ids.UserID, string) error { return nil }
func (c *invitationCalendar) Busy(context.Context, ids.UserID, string, string, time.Time, time.Time) ([]connector.CalendarInterval, error) {
	return c.busy, nil
}

func (c *invitationCalendar) Save(_ context.Context, _ ids.UserID, _ string, in connector.CalendarAppointment) (connector.CalendarReceipt, error) {
	c.requests = append(c.requests, in)
	if c.fail {
		return connector.CalendarReceipt{}, connector.ErrUnreachable
	}
	return connector.CalendarReceipt{EventID: in.RequestID, UID: in.RequestID + "@calendar.test", URL: "https://calendar.example.test/event"}, nil
}

func (c *invitationCalendar) Cancel(context.Context, ids.UserID, string, string, string) error {
	c.canceled++
	return nil
}

func TestInvitationDeliveryKeepsOneActivityAndOneProviderIdentity(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	clock := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	calendar := &invitationCalendar{fail: true}
	store := e.store(nil).WithClock(func() time.Time { return clock }).WithWorkingHours(func(context.Context, ids.UserID) (WorkingHours, error) { return fallbackWorkingHours(), nil }).WithSchedulingCalendar(calendar).WithMeetingVault(keyvault.NewMemory()).WithPublicBaseURL("https://crm.example.com")
	profile := defaultSchedulingProfile()
	profile.Provider = "gcal"
	profile.Enabled = true
	name := "Test Host"
	profile.HostName = &name
	profile.BufferMinutes = 0
	if _, err := store.SaveSchedulingProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	contact := ids.NewV7()
	args := schedulingArgs{}
	if _, err := e.owner.Exec(ctx, `INSERT INTO contact(id,full_name,source,captured_by)VALUES(`+args.add(contact)+`,'Guest','manual','human:test')`, args...); err != nil {
		t.Fatal(err)
	}
	in := crmcontracts.MeetingInvitationRequest{ContactId: crmcontracts.Id(contact), AttendeeEmail: "guest@example.test", Subject: "Project meeting", Start: clock.Add(3 * time.Hour), End: clock.Add(3*time.Hour + 30*time.Minute)}
	invited, err := store.CreateInvitation(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if invited.Status != "pending" || invited.ManagementToken == nil {
		t.Fatalf("unexpected pending result: %+v", invited)
	}
	if err := store.DeliverInvitations(ctx); err != nil {
		t.Fatal(err)
	}
	pending, err := store.Invitation(ctx, ids.UUID(invited.Id))
	if err != nil || pending.Status != "pending" {
		t.Fatalf("uncertain send must stay pending: %+v %v", pending, err)
	}
	if _, err := store.CreateInvitation(ctx, in); err == nil {
		t.Fatal("uncertain provider result released the reservation")
	}
	calendar.fail = false
	clock = clock.Add(2 * time.Minute)
	if err := store.DeliverInvitations(ctx); err != nil {
		t.Fatal(err)
	}
	booked, err := store.Invitation(ctx, ids.UUID(invited.Id))
	if err != nil || booked.Status != "confirmed" {
		t.Fatalf("confirmation: %+v %v", booked, err)
	}
	if len(calendar.requests) != 2 || calendar.requests[0].RequestID != calendar.requests[1].RequestID {
		t.Fatal("retry changed the durable provider identity")
	}
	next := in
	next.Start = in.End
	next.End = in.End.Add(30 * time.Minute)
	if _, err := store.CreateInvitation(ctx, next); err != nil {
		t.Fatalf("adjacent half-hour slot should be free: %v", err)
	}
	canceled, err := store.ChangeInvitation(ctx, ids.UUID(invited.Id), crmcontracts.MeetingInvitationChange{Action: "cancel", Version: booked.Version})
	if err != nil || canceled.Status != "canceling" {
		t.Fatalf("cancel: %+v %v", canceled, err)
	}
	if err := store.DeliverInvitations(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := store.Invitation(ctx, ids.UUID(invited.Id))
	if err != nil || state.Status != "canceled" || calendar.canceled != 1 {
		t.Fatalf("canceled: %+v %v calls=%d", state, err, calendar.canceled)
	}
	if _, err := store.CreateInvitation(ctx, in); err != nil {
		t.Fatalf("acknowledged cancellation did not release slot: %v", err)
	}
	var count int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM activity WHERE kind='meeting'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("delivery retry duplicated the CRM meeting: %d", count)
	}
}

func TestInvitationPolicyRejectsBusyAndOutOfHoursBeforeWriting(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	now := monday(6)
	calendar := &invitationCalendar{busy: []connector.CalendarInterval{{Start: monday(10), End: monday(12)}}}
	store := e.store(nil).WithClock(func() time.Time { return now }).WithWorkingHours(func(context.Context, ids.UserID) (WorkingHours, error) { return fallbackWorkingHours(), nil }).WithSchedulingCalendar(calendar).WithMeetingVault(keyvault.NewMemory()).WithPublicBaseURL("https://crm.example.com")
	profile := defaultSchedulingProfile()
	profile.Provider = "gcal"
	profile.Enabled = true
	name := "Test Host"
	profile.HostName = &name
	if _, err := store.SaveSchedulingProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	for _, start := range []time.Time{monday(3), monday(10), monday(18)} {
		_, err := store.CreateInvitation(ctx, crmcontracts.MeetingInvitationRequest{ContactId: crmcontracts.Id(ids.NewV7()), AttendeeEmail: "guest@example.test", Subject: "Meeting", Start: start, End: start.Add(30 * time.Minute)})
		if err == nil {
			t.Fatalf("accepted unavailable time %v", start)
		}
		var conflict *SlotTakenError
		var invalid *SchedulingArgumentError
		if !errors.As(err, &conflict) && !errors.As(err, &invalid) {
			t.Fatalf("unexpected refusal %v", err)
		}
	}
	var count int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM meeting_invitation`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid requests wrote %d invitations", count)
	}
}

func (c *invitationCalendar) Lookup(context.Context, ids.UserID, string, connector.CalendarAppointment) (*connector.CalendarReceipt, error) {
	return nil, c.lookupErr
}

func (c *invitationCalendar) CheckRecipient(context.Context, ids.UserID, ids.UUID, string) error {
	return nil
}

func (c *invitationCalendar) List(context.Context, ids.UserID, string) ([]connector.CalendarOption, error) {
	return []connector.CalendarOption{{ID: "primary", Name: "Calendar", Writable: true, Primary: true}}, nil
}

func (c *invitationCalendar) Inspect(context.Context, ids.UserID, string, string, string) (connector.CalendarState, error) {
	if c.inspectErr != nil {
		return connector.CalendarState{}, c.inspectErr
	}
	if len(c.requests) == 0 {
		return connector.CalendarState{}, connector.ErrUnreachable
	}
	last := c.requests[len(c.requests)-1]
	return connector.CalendarState{Start: last.Start, End: last.End}, nil
}
