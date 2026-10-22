// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestBookingProfileReadsAccountNameBeforeItsLinkExists(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil).WithPublicBaseURL("https://crm.example.test")
	profile, err := store.SchedulingProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.HostName == nil || *profile.HostName != "Rep" || profile.Slug != nil || profile.PublicUrl != nil {
		t.Fatalf("uncreated profile: %+v", profile)
	}
	legacyName := "Unrelated name"
	profile.HostName = &legacyName
	saved, err := store.SaveSchedulingProfile(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	if saved.HostName == nil || *saved.HostName != "Rep" || saved.Slug == nil || saved.PublicUrl == nil || saved.Enabled {
		t.Fatalf("saved profile: %+v", saved)
	}
	var storedName bool
	args := schedulingArgs{}
	err = e.owner.QueryRow(ctx, `SELECT scheduling_policy ? 'host_name' FROM booking_page WHERE host_user_id=`+args.add(e.rep), args...).Scan(&storedName)
	if err != nil {
		t.Fatal(err)
	}
	if storedName {
		t.Fatal("booking policy duplicated the account name")
	}
	// The first save of a switched-off page is still a write, so it leaves the
	// audit row and the event every booking page write leaves.
	var page ids.UUID
	args = schedulingArgs{}
	if err := e.owner.QueryRow(ctx, `SELECT id FROM booking_page WHERE slug=`+args.add(*saved.Slug), args...).Scan(&page); err != nil {
		t.Fatal(err)
	}
	var audits, events int
	args = schedulingArgs{}
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type='booking_page' AND action='create' AND entity_id=`+args.add(page), args...).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	args = schedulingArgs{}
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='booking_page.updated' AND envelope->'entity'->>'id'=`+args.add(page.String()), args...).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || events != 1 {
		t.Fatalf("first save left %d create audits and %d booking_page.updated events, want one of each", audits, events)
	}
}

func TestBookingProfileIgnoresALegacyStoredName(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil).WithPublicBaseURL("https://crm.example.test")
	profile, err := store.SaveSchedulingProfile(ctx, defaultSchedulingProfile())
	if err != nil {
		t.Fatal(err)
	}
	// No current writer persists this field; reproduce a previously saved policy.
	args := schedulingArgs{}
	_, err = e.owner.Exec(ctx, `UPDATE booking_page SET scheduling_policy = scheduling_policy || '{"host_name":"Obsolete name"}'::jsonb WHERE slug=`+args.add(*profile.Slug), args...)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = store.SchedulingProfile(ctx)
	if err != nil || profile.HostName == nil || *profile.HostName != "Rep" {
		t.Fatalf("legacy profile: %+v, %v", profile, err)
	}
}

// authorityCalendar counts the provider checks a save asks for. checkErr stands
// in for a lapsed grant.
type authorityCalendar struct {
	invitationCalendar
	checks   int
	checkErr error
}

func (c *authorityCalendar) Check(context.Context, ids.UserID, string) error {
	c.checks++
	return c.checkErr
}

func (c *authorityCalendar) List(context.Context, ids.UserID, string) ([]connector.CalendarOption, error) {
	return []connector.CalendarOption{
		{ID: "cal-own", Name: "Own", Writable: true, Primary: true},
		{ID: "cal-team", Name: "Team", Writable: true},
	}, nil
}

// liveBookingPage switches a page on through the calendar check, as the first
// enable must, and hands back what the server stored.
func liveBookingPage(t *testing.T, e *sendEnv, calendar *authorityCalendar) crmcontracts.SchedulingProfile {
	t.Helper()
	profile := defaultSchedulingProfile()
	profile.Provider, profile.Enabled = "gcal", true
	saved, err := e.store(nil).WithPublicBaseURL("https://crm.example.com").WithSchedulingCalendar(calendar).SaveSchedulingProfile(e.as(principal.RowScopeAll), profile)
	if err != nil {
		t.Fatal(err)
	}
	if calendar.checks != 1 || saved.CalendarId != "cal-own" {
		t.Fatalf("first enable: %d checks, calendar %q; want one check resolving primary to cal-own", calendar.checks, saved.CalendarId)
	}
	return saved
}

// A live page whose calendar grant has lapsed still saves an edit the calendar
// never sees. Nothing is asked of the provider, so a store with no calendar
// bound at all saves it too.
func TestALiveBookingPageSavesItsLimitsWithoutAskingTheCalendar(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	calendar := &authorityCalendar{}
	saved := liveBookingPage(t, e, calendar)

	calendar.checkErr = connector.ErrUnreachable
	saved.NoticeMinutes, saved.BufferMinutes = 60, 0
	empty := []string{}
	saved.BlockingCalendars = &empty
	bound := e.store(nil).WithPublicBaseURL("https://crm.example.com").WithSchedulingCalendar(calendar)
	if _, err := bound.SaveSchedulingProfile(ctx, saved); err != nil {
		t.Fatalf("a notice edit on a live page asked the lapsed calendar: %v", err)
	}
	unbound := e.store(nil).WithPublicBaseURL("https://crm.example.com")
	saved.HorizonDays = 14
	after, err := unbound.SaveSchedulingProfile(ctx, saved)
	if err != nil {
		t.Fatalf("a horizon edit needed a calendar binding: %v", err)
	}
	if calendar.checks != 1 || after.NoticeMinutes != 60 || after.HorizonDays != 14 || !after.Enabled {
		t.Fatalf("after two limit edits: %d checks, stored %+v; want no new check and both edits kept", calendar.checks, after)
	}
}

// Pointing a live page at another calendar, or at new blocking calendars, is
// what the check vouches for. So it runs again and its refusal stands.
func TestALiveBookingPageChecksTheCalendarItIsPointedAt(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	calendar := &authorityCalendar{}
	saved := liveBookingPage(t, e, calendar)
	store := e.store(nil).WithPublicBaseURL("https://crm.example.com").WithSchedulingCalendar(calendar)

	blocking := []string{"cal-gone"}
	saved.BlockingCalendars = &blocking
	var refused *SchedulingArgumentError
	if _, err := store.SaveSchedulingProfile(ctx, saved); !errors.As(err, &refused) || refused.Field != "blocking_calendars" {
		t.Fatalf("a new blocking calendar the connection lacks saved: %v", err)
	}
	saved.BlockingCalendars = nil

	calendar.checkErr = connector.ErrUnreachable
	saved.CalendarId = "cal-team"
	if _, err := store.SaveSchedulingProfile(ctx, saved); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("moving to another calendar on a lapsed grant answered %v, want the provider's refusal", err)
	}
	if calendar.checks != 3 {
		t.Fatalf("%d checks, want one per calendar change on top of the first enable", calendar.checks)
	}
}

// Switching a page off asks nothing; switching it back on is a fresh
// publication and checks the calendar again.
func TestSwitchingABookingPageBackOnChecksTheCalendarAgain(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	calendar := &authorityCalendar{}
	saved := liveBookingPage(t, e, calendar)
	store := e.store(nil).WithPublicBaseURL("https://crm.example.com").WithSchedulingCalendar(calendar)

	calendar.checkErr = connector.ErrUnreachable
	saved.Enabled = false
	paused, err := store.SaveSchedulingProfile(ctx, saved)
	if err != nil {
		t.Fatalf("pausing on a lapsed grant: %v", err)
	}
	paused.Enabled = true
	if _, err := store.SaveSchedulingProfile(ctx, paused); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("re-enabling on a lapsed grant answered %v, want the provider's refusal", err)
	}
	if calendar.checks != 2 {
		t.Fatalf("%d checks, want the first enable and the re-enable only", calendar.checks)
	}
}

// A host who picks a provider before connecting it gets the sentence the
// no-provider case already gives, naming the provider field, not a server error.
func TestEnablingBookingsWithoutAUsableCalendarNamesTheProvider(t *testing.T) {
	for name, calendar := range map[string]*invitationCalendar{
		"no connection":              {checkErr: connector.ErrAuthRejected},
		"a connection since revoked": {listErr: connector.ErrAuthRejected},
	} {
		e := setupSend(t)
		store := e.store(nil).WithPublicBaseURL("https://crm.example.test").WithSchedulingCalendar(calendar)
		profile := defaultSchedulingProfile()
		profile.Enabled = true
		profile.Provider = "gcal"

		_, err := store.SaveSchedulingProfile(e.as(principal.RowScopeAll), profile)

		var refusal *SchedulingArgumentError
		if !errors.As(err, &refusal) || refusal.Field != "provider" || refusal.Code != "required" {
			t.Errorf("%s answered %v, want a refusal naming provider", name, err)
		}
	}
}

// The contract lists the providers a profile may name; a disabled profile is
// held to it too, so an unknown value is never stored.
func TestSavingAProfileRefusesAProviderOutsideTheContract(t *testing.T) {
	e := setupSend(t)
	store := e.store(nil).WithPublicBaseURL("https://crm.example.test")
	profile := defaultSchedulingProfile()
	profile.Enabled = false
	profile.Provider = "zoom"

	_, err := store.SaveSchedulingProfile(e.as(principal.RowScopeAll), profile)

	var refusal *SchedulingArgumentError
	if !errors.As(err, &refusal) || refusal.Field != "provider" || refusal.Code != "invalid" {
		t.Fatalf("saving provider zoom answered %v, want an invalid refusal naming provider", err)
	}
}
