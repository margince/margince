// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Scheduling is calendar access: booking another host's calendar is the
// admin's alone — an unbounded row scope reads every calendar and writes
// none but its own — and the availability busy-read shows a caller only
// the meetings their timeline would; a stranger's calendar never leaks
// through free/busy.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bookableStore books through a connected calendar, with a booking page on for
// each host.
func bookableStore(t *testing.T, e *Env, hosts ...ids.UUID) *activities.Store {
	t.Helper()
	store := e.Activities.WithSchedulingCalendar(&apptest.BookingCalendar{SchedulingCalendar: compose.SchedulingCalendarForTest(e.Pool)}).WithMeetingVault(keyvault.NewMemory()).
		WithPublicBaseURL("https://crm.example.test").
		WithWorkingHours(func(context.Context, ids.UserID) (activities.WorkingHours, error) {
			return activities.WorkingHours{StartMinute: 540, EndMinute: 1020, Days: []int{1, 2, 3, 4, 5}, Location: time.UTC}, nil
		})
	for _, host := range hosts {
		if _, err := store.SaveSchedulingProfile(e.As(host, nil, SchedulerPerms), crmcontracts.SchedulingProfile{
			Enabled: true, Provider: "gcal", CalendarId: "work", DurationMinutes: 30, HorizonDays: 30, Title: "Meeting",
		}); err != nil {
			t.Fatalf("enabling %s's booking page: %v", host, err)
		}
	}
	return store
}

// inviteeContact is a contact owned by owner with an address to invite.
func inviteeContact(t *testing.T, e *Env, name string, owner ids.UUID) ids.UUID {
	t.Helper()
	id := e.SeedContact(t, name, &owner)
	e.WsExec(t, `INSERT INTO contact_email(contact_id,email,is_primary,source,captured_by) VALUES ($1,$2,true,'manual','human:test')`, id, id.String()+"@client.test")
	return id
}

func TestBookingAnotherHostNeedsTheAdminRole(t *testing.T) {
	e := Setup(t)
	slotStart := apptest.NextBookableMorning()
	store := bookableStore(t, e, e.Rep1, e.Rep2)

	// Every booking names an invitee, because a meeting with nobody is refused
	// before the calendar rules run. The contact is incidental here — what
	// this test is about is whose calendar, not whose record.
	attendee := inviteeContact(t, e, "Calendar Client", e.Rep1)
	links := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: attendee}}

	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, SchedulerPerms)
	if _, err := store.BookMeeting(rep1, activities.BookMeetingInput{
		Host: ids.From[ids.UserKind](e.Rep1), Start: slotStart, End: slotStart.Add(time.Hour), Links: links,
	}); err != nil {
		t.Fatalf("self-booking: %v", err)
	}
	if _, err := store.BookMeeting(rep1, activities.BookMeetingInput{
		Host: ids.From[ids.UserKind](e.Rep2), Start: slotStart, End: slotStart.Add(time.Hour), Links: links,
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("booking rep2's calendar as rep1 → %v, want ErrPermissionDenied", err)
	}
	// An unbounded row scope is not a delegate: ops reads every calendar
	// and still books only its own.
	ops := e.As(ids.NewV7(), nil, OpsPerms)
	if _, err := store.BookMeeting(ops, activities.BookMeetingInput{
		Host: ids.From[ids.UserKind](e.Rep2), Start: slotStart, End: slotStart.Add(time.Hour), Links: links,
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("booking rep2's calendar as ops → %v, want ErrPermissionDenied", err)
	}
	// The admin may book on behalf — and hits the conflict guard like
	// anyone else.
	admin := e.Admin()
	if _, err := store.BookMeeting(admin, activities.BookMeetingInput{
		Host: ids.From[ids.UserKind](e.Rep2), Start: slotStart, End: slotStart.Add(time.Hour), Links: links,
	}); err != nil {
		t.Fatalf("admin booking for rep2: %v", err)
	}
	var slotTaken *activities.SlotTakenError
	if _, err := store.BookMeeting(rep1, activities.BookMeetingInput{
		Host: ids.From[ids.UserKind](e.Rep1), Start: slotStart, End: slotStart.Add(time.Hour), Links: links,
	}); !errors.As(err, &slotTaken) {
		t.Fatalf("double self-booking → %v, want SlotTakenError", err)
	}
}

func TestAvailabilityBusyReadHonorsRowScope(t *testing.T) {
	e := Setup(t)
	slotStart := apptest.NextBookableMorning()
	store := bookableStore(t, e, e.Rep1)

	// rep1's meeting is linked to a contact rep1 captured PRIVATELY — visible
	// to rep1 alone. A contact who is merely owned is readable by every seat
	// with the grant, so capture privacy is what keeps the meeting out of a
	// colleague's row scope.
	target := inviteeContact(t, e, "Scoped Client", e.Rep1)
	e.MakeCapturePrivate(t, "contact", target, e.Rep1)
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, SchedulerPerms)
	if _, err := store.BookMeeting(rep1, activities.BookMeetingInput{
		Host: ids.From[ids.UserKind](e.Rep1), Start: slotStart, End: slotStart.Add(time.Hour),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: target}},
	}); err != nil {
		t.Fatalf("booking: %v", err)
	}

	windowFrom, windowTo := slotStart.Add(-2*time.Hour), slotStart.Add(6*time.Hour)
	proposes := func(ctx context.Context) bool {
		t.Helper()
		slots, truncated, err := e.Activities.Availability(ctx, ids.From[ids.UserKind](e.Rep1), windowFrom, windowTo, time.Hour)
		if err != nil {
			t.Fatalf("availability: %v", err)
		}
		// An answer cut short by the slot cap would make "the slot is absent"
		// ambiguous — absent because the row scope hid the meeting, or absent
		// because the walk stopped early. This window is far too small for that,
		// and asserting it keeps the row-scope conclusion below sound.
		if truncated {
			t.Fatalf("the %v window hit the slot cap; this test cannot tell a hidden slot from a dropped one",
				windowTo.Sub(windowFrom))
		}
		for _, s := range slots {
			if s.Start.Equal(slotStart) {
				return true
			}
		}
		return false
	}

	// The captor sees the block; a teammate and a rep from the other team
	// both see rep1's calendar as free at that slot — the meeting is outside
	// their row scope and must not leak through free/busy. Capture privacy
	// is not team-shaped, so the teammate is hidden from it too.
	if proposes(rep1) {
		t.Fatal("the captor still sees the booked slot as free")
	}
	teammate := e.As(e.Rep2, []ids.UUID{e.Team1}, SchedulerPerms)
	if !proposes(teammate) {
		t.Fatal("a teammate can see the busy block — free/busy leaks the private meeting")
	}
	stranger := e.As(e.Rep3, []ids.UUID{e.Team2}, SchedulerPerms)
	if !proposes(stranger) {
		t.Fatal("out-of-scope caller can see the busy block — free/busy leaks the hidden meeting")
	}
}
