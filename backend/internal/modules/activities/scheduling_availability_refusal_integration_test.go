// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Every availability door reads the host's calendar. A host whose connection
// is gone is refused as a request they can act on, never answered with a 500.
func TestAvailabilityWithoutAUsableCalendarIsRefusedAtEveryDoor(t *testing.T) {
	for name, fail := range map[string]func(*invitationCalendar){
		"check": func(c *invitationCalendar) { c.checkErr = connector.ErrAuthRejected },
		"busy":  func(c *invitationCalendar) { c.busyErr = connector.ErrAuthRejected },
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
		fail(f.calendar)
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
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s via %s answered %d %s, want 422", door, name, rec.Code, rec.Body.String())
			}
		}
	}
}
