// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Every availability door reads the host's calendar. A gone connection is a
// request the host can act on. A calendar the provider could not report is
// named as the problem. Neither is a bare 500.
func TestAvailabilityWithoutAUsableCalendarIsRefusedAtEveryDoor(t *testing.T) {
	for name, tc := range map[string]struct {
		fail func(*invitationCalendar)
		want int
	}{
		"check":      {func(c *invitationCalendar) { c.checkErr = connector.ErrAuthRejected }, http.StatusUnprocessableEntity},
		"busy":       {func(c *invitationCalendar) { c.busyErr = connector.ErrAuthRejected }, http.StatusUnprocessableEntity},
		"unreadable": {func(c *invitationCalendar) { c.busyErr = connector.NewCalendarUnavailable("primary", "notFound", nil) }, http.StatusBadGateway},
		"unreached": {func(c *invitationCalendar) {
			c.busyErr = fmt.Errorf("calendar: provider status 503: %w", connector.ErrUnreachable)
		}, http.StatusBadGateway},
		"our fault": {func(c *invitationCalendar) { c.busyErr = errors.New("keyvault: key resolution failed") }, http.StatusInternalServerError},
	} {
		f := newInvitationFixture(t)
		hostName := "Test Host"
		profile := defaultSchedulingProfile()
		profile.Provider, profile.Enabled, profile.HostName, profile.BufferMinutes = "gcal", true, &hostName, 0
		saved, err := f.store.SaveSchedulingProfile(f.ctx, profile)
		if err != nil {
			t.Fatal(err)
		}
		booked, err := f.store.CreateInvitation(f.ctx, f.request)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.DeliverInvitations(f.ctx); err != nil {
			t.Fatal(err)
		}
		link, err := f.store.CreateProposal(f.ctx, crmcontracts.MeetingProposalRequest{ContactId: f.request.ContactId, AttendeeEmail: f.request.AttendeeEmail, Subject: f.request.Subject, DurationMinutes: 30})
		if err != nil {
			t.Fatal(err)
		}
		tc.fail(f.calendar)
		h := Handlers{store: f.store}
		from, to := monday(8), monday(18)
		doors := map[string]func(*httptest.ResponseRecorder, *http.Request){
			"invitation": func(w *httptest.ResponseRecorder, r *http.Request) {
				h.GetMeetingAvailability(w, r, booked.Id, crmcontracts.GetMeetingAvailabilityParams{From: from, To: to})
			},
			"management link": func(w *httptest.ResponseRecorder, r *http.Request) {
				h.GetPublicMeetingAvailability(w, r, *booked.ManagementToken, crmcontracts.GetPublicMeetingAvailabilityParams{From: from, To: to})
			},
			"proposal link": func(w *httptest.ResponseRecorder, r *http.Request) {
				h.GetPublicProposalAvailability(w, r, strings.Split(link.URL, "proposal-")[1], crmcontracts.GetPublicProposalAvailabilityParams{From: from, To: to})
			},
			"booking page": func(w *httptest.ResponseRecorder, r *http.Request) {
				h.GetPublicAvailability(w, r, *saved.Slug, crmcontracts.GetPublicAvailabilityParams{From: from, To: to})
			},
		}
		for door, call := range doors {
			// A booked meeting's own doors read only the busy times.
			if name == "check" && (door == "invitation" || door == "management link") {
				continue
			}
			rec := httptest.NewRecorder()
			call(rec, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(f.ctx))
			if rec.Code != tc.want {
				t.Errorf("%s via %s answered %d %s, want %d", door, name, rec.Code, rec.Body.String(), tc.want)
			}
			if tc.want == http.StatusBadGateway && !strings.Contains(rec.Body.String(), connector.CalendarUnavailableCode) {
				t.Errorf("%s via %s does not name the calendar as the problem: %s", door, name, rec.Body.String())
			}
		}
	}
}

// The host asking about their own calendars learns which selected calendar
// could not be read. A public door, a colleague, an agent acting for the host
// and a system job never see it.
func TestOnlyTheHostIsToldWhichCalendarCouldNotBeRead(t *testing.T) {
	const unreadable = "team-holidays@group.calendar.test"
	f := newInvitationFixture(t)
	hostName := "Test Host"
	profile := defaultSchedulingProfile()
	profile.Provider, profile.Enabled, profile.HostName = "gcal", true, &hostName
	saved, err := f.store.SaveSchedulingProfile(f.ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	link, err := f.store.CreateProposal(f.ctx, crmcontracts.MeetingProposalRequest{ContactId: f.request.ContactId, AttendeeEmail: f.request.AttendeeEmail, Subject: f.request.Subject, DurationMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	f.calendar.busyErr = connector.NewCalendarUnavailable(unreadable, "notFound", nil)
	h := Handlers{store: f.store}
	from, to := monday(8), monday(18)
	reliable := true
	host, ok := principal.Actor(f.ctx)
	if !ok {
		t.Fatal("the fixture reads as nobody")
	}
	hostID := openapi_types.UUID(host.UserID)
	colleague := host
	colleague.UserID, colleague.ID = ids.NewV7(), "human:colleague"
	answer := func(call func(*httptest.ResponseRecorder, *http.Request), ctx context.Context) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		call(rec, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(ctx))
		return rec
	}
	own := answer(func(w *httptest.ResponseRecorder, r *http.Request) {
		h.GetAvailability(w, r, crmcontracts.GetAvailabilityParams{From: from, To: to, Reliable: &reliable})
	}, f.ctx)
	if own.Code != http.StatusBadGateway || !strings.Contains(own.Body.String(), `"calendar":"`+unreadable+`"`) {
		t.Errorf("the host's own read answered %d %s, want calendar_unavailable naming the calendar", own.Code, own.Body.String())
	}
	ownRead := func(w *httptest.ResponseRecorder, r *http.Request) {
		h.GetAvailability(w, r, crmcontracts.GetAvailabilityParams{From: from, To: to, Reliable: &reliable})
	}
	agent := host
	agent.Type, agent.ID, agent.PassportID, agent.OnBehalfOf = principal.PrincipalAgent, "agent:passport", ids.NewV7(), host.UserID
	system := host
	system.Type, system.ID = principal.PrincipalSystem, "system:test"
	for door, call := range map[string]func(*httptest.ResponseRecorder, *http.Request){
		"a colleague asking about the host": func(w *httptest.ResponseRecorder, r *http.Request) {
			h.GetAvailability(w, r, crmcontracts.GetAvailabilityParams{From: from, To: to, Reliable: &reliable, HostUserId: &hostID})
		},
		"an agent passport acting for the host": ownRead,
		"a system job":                          ownRead,
		"the booking page": func(w *httptest.ResponseRecorder, r *http.Request) {
			h.GetPublicAvailability(w, r, *saved.Slug, crmcontracts.GetPublicAvailabilityParams{From: from, To: to})
		},
		"the proposal link": func(w *httptest.ResponseRecorder, r *http.Request) {
			h.GetPublicProposalAvailability(w, r, strings.Split(link.URL, "proposal-")[1], crmcontracts.GetPublicProposalAvailabilityParams{From: from, To: to})
		},
	} {
		ctx := f.ctx
		switch door {
		case "a colleague asking about the host":
			ctx = principal.WithActor(f.ctx, colleague)
		case "an agent passport acting for the host":
			ctx = principal.WithActor(f.ctx, agent)
		case "a system job":
			ctx = principal.WithActor(f.ctx, system)
		}
		rec := answer(call, ctx)
		if strings.Contains(rec.Body.String(), unreadable) {
			t.Errorf("%s answered %d %s, which names the host's calendar", door, rec.Code, rec.Body.String())
		}
	}
}

// The settings page reads the host's own availability from now to a day past
// the saved notice, which is the request it sends. Under the default two
// hours' notice that window still reaches the calendars, so they are named.
func TestTheSettingsCheckNamesTheUnreadableCalendarUnderTheDefaultNotice(t *testing.T) {
	const unreadable = "team-holidays@group.calendar.test"
	f := newInvitationFixture(t)
	profile := defaultSchedulingProfile()
	profile.Provider = "gcal"
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	f.calendar.busyErr = connector.NewCalendarUnavailable(unreadable, "notFound", nil)
	h := Handlers{store: f.store}
	reliable := true
	from := f.now
	to := from.Add(time.Duration(profile.NoticeMinutes)*time.Minute + 24*time.Hour)
	rec := httptest.NewRecorder()
	h.GetAvailability(rec, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(f.ctx),
		crmcontracts.GetAvailabilityParams{From: from, To: to, Reliable: &reliable})
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), `"calendar":"`+unreadable+`"`) {
		t.Errorf("the settings check answered %d %s, want the unreadable calendar named", rec.Code, rec.Body.String())
	}
}
