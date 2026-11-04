// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type bookingFixture struct {
	e        *sendEnv
	ctx      context.Context
	store    *Store
	calendar *invitationCalendar
	clock    time.Time
}

func setupBooking(t *testing.T) bookingFixture {
	t.Helper()
	e := setupSend(t)
	f := bookingFixture{e: e, ctx: e.as(principal.RowScopeAll), calendar: &invitationCalendar{invitees: map[ids.UUID]string{}}, clock: time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)}
	f.store = e.store(nil).WithClock(func() time.Time { return f.clock }).
		WithWorkingHours(func(context.Context, ids.UserID) (WorkingHours, error) { return fallbackWorkingHours(), nil }).
		WithSchedulingCalendar(f.calendar).WithMeetingVault(keyvault.NewMemory()).WithPublicBaseURL("https://crm.example.com")
	profile := defaultSchedulingProfile()
	profile.Provider, profile.Enabled, profile.BufferMinutes = "gcal", true, 0
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f bookingFixture) contactWithEmail(t *testing.T, email string) ids.UUID {
	t.Helper()
	contact := ids.NewV7()
	if _, err := f.e.owner.Exec(f.ctx, `INSERT INTO contact(id,full_name,source,captured_by) VALUES ($1,'Guest','manual','human:test')`, contact); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.owner.Exec(f.ctx, `INSERT INTO contact_email(contact_id,email,is_primary,source,captured_by) VALUES ($1,$2,true,'manual','human:test')`, contact, email); err != nil {
		t.Fatal(err)
	}
	f.calendar.invitees[contact] = email
	return contact
}

func (f bookingFixture) booking(links ...ActivityLinkInput) BookMeetingInput {
	start := f.clock.Add(3 * time.Hour)
	return BookMeetingInput{Host: ids.From[ids.UserKind](f.e.rep), Start: start, End: start.Add(30 * time.Minute), Links: links}
}

func TestABookingQueuesOneInvitationForEveryAttendee(t *testing.T) {
	f := setupBooking(t)
	contact := f.contactWithEmail(t, "guest@example.test")
	f.calendar.contacts = map[string]ids.UUID{"guest@example.test": contact}
	in := f.booking(ActivityLinkInput{EntityType: linkEntityContact, EntityID: contact})
	in.AttendeeEmails = []string{"Second@Example.test", "guest@example.test", "third@example.test", "second@example.test"}

	meeting, err := f.store.BookMeeting(f.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if meeting.Subject == nil || *meeting.Subject != DefaultMeetingSubject {
		t.Errorf("subject = %v, want the default %q", meeting.Subject, DefaultMeetingSubject)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.calendar.requests) != 1 {
		t.Fatalf("provider saves = %d, want one event for the whole booking", len(f.calendar.requests))
	}
	want := []string{"guest@example.test", "second@example.test", "third@example.test"}
	if got := f.calendar.requests[0].Attendees; !slices.Equal(got, want) {
		t.Errorf("event attendees = %v, want %v", got, want)
	}
	rows, err := f.e.owner.Query(f.ctx, `SELECT role, user_id IS NOT NULL, contact_id IS NOT NULL, coalesce(address,''), coalesce(display_name,'<none>') FROM activity_participant
		WHERE activity_id = $1 AND role IN ('organizer','attendee') ORDER BY role DESC, address NULLS FIRST`, meeting.Id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var role, address, name string
		var user, byContact bool
		if err := rows.Scan(&role, &user, &byContact, &address, &name); err != nil {
			t.Fatal(err)
		}
		switch {
		case user:
			address = "user"
		case byContact:
			address = "contact"
		}
		got = append(got, role+":"+address+":"+name)
	}
	// An attendee carries its record's name; a host-named address has none to give.
	if want := []string{"organizer:user:<none>", "attendee:contact:Contact guest@example.test", "attendee:second@example.test:", "attendee:third@example.test:"}; !slices.Equal(got, want) {
		t.Errorf("participants = %v, want %v", got, want)
	}
}

func TestABookingNeedsAnInvitee(t *testing.T) {
	f := setupBooking(t)
	deal := f.booking(ActivityLinkInput{EntityType: linkEntityDeal, EntityID: ids.NewV7()})
	deal.AttendeeEmails = []string{"nobody@example.test"}
	_, err := f.store.BookMeeting(f.ctx, deal)
	if refusal, ok := errors.AsType[*SchedulingArgumentError](err); !ok || refusal.Code != "invitee_required" {
		t.Fatalf("a booking naming nobody answered %v, want invitee_required", err)
	}

	known := f.contactWithEmail(t, "known@example.test")
	f.calendar.contacts = map[string]ids.UUID{"known@example.test": known}
	pipeline, stage, dealID := ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, seed := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO pipeline (id, name) VALUES ($1, 'Booking pipeline')`, []any{pipeline}},
		{`INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, []any{stage, pipeline}},
		{`INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by) VALUES ($1, 'Renewal', 'open', $2, $3, $4, 'seed', 'system')`, []any{dealID, f.e.rep, pipeline, stage}},
	} {
		if _, err := f.e.owner.Exec(f.ctx, seed.sql, seed.args...); err != nil {
			t.Fatal(err)
		}
	}
	actor, _ := principal.Actor(f.ctx)
	actor.Permissions.Objects["deal"] = principal.ObjectGrant{Read: true}
	f.ctx = principal.WithActor(f.ctx, actor)
	byAddress := f.booking(ActivityLinkInput{EntityType: linkEntityDeal, EntityID: dealID})
	byAddress.AttendeeEmails = []string{"nobody@example.test", "known@example.test"}
	if _, err := f.store.BookMeeting(f.ctx, byAddress); err != nil {
		t.Fatalf("an address that resolves to a contact was refused: %v", err)
	}
	if got := f.lastAppointment(t); got.ContactID != known || got.Attendees[0] != "known@example.test" {
		t.Errorf("invitee = %s %v, want the contact the address resolved to", got.ContactID, got.Attendees)
	}

	lead := ids.NewV7()
	if _, err := f.e.owner.Exec(f.ctx, `INSERT INTO lead(id,full_name,email,source,captured_by) VALUES ($1,'Lead','lead@example.test','manual','human:test')`, lead); err != nil {
		t.Fatal(err)
	}
	f.calendar.invitees[lead] = "lead@example.test"
	byLead := f.booking(ActivityLinkInput{EntityType: linkEntityLead, EntityID: lead})
	byLead.Start, byLead.End = byLead.Start.Add(time.Hour), byLead.End.Add(time.Hour)
	if _, err := f.store.BookMeeting(f.ctx, byLead); err != nil {
		t.Fatalf("a lead link was refused: %v", err)
	}
	if got := f.lastAppointment(t); got.LeadID != lead || got.Attendees[0] != "lead@example.test" {
		t.Errorf("lead invitee = %s %v, want the lead at its own address", got.LeadID, got.Attendees)
	}
}

func (f bookingFixture) lastAppointment(t *testing.T) connector.CalendarAppointment {
	t.Helper()
	var appointment connector.CalendarAppointment
	if err := f.e.owner.QueryRow(f.ctx, `SELECT appointment FROM meeting_invitation ORDER BY created_at DESC, activity_id DESC LIMIT 1`).Scan(&appointment); err != nil {
		t.Fatal(err)
	}
	return appointment
}

// A lead attendee's row is an address row, read without the lead's scope, so
// it carries no name even when the roster offers one.
func TestALeadAttendeeRowCarriesNoName(t *testing.T) {
	f := setupBooking(t)
	contact := f.contactWithEmail(t, "guest@example.test")
	f.calendar.leads = map[string]ids.UUID{"lead@example.test": ids.NewV7()}
	in := f.booking(ActivityLinkInput{EntityType: linkEntityContact, EntityID: contact})
	in.AttendeeEmails = []string{"lead@example.test"}
	meeting, err := f.store.BookMeeting(f.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	var name *string
	if err := f.e.owner.QueryRow(f.ctx, `SELECT display_name FROM activity_participant WHERE activity_id = $1 AND address = 'lead@example.test'`, meeting.Id).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name == nil || *name != "" {
		t.Errorf("the lead's row is named %v, want empty", name)
	}
}
