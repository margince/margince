// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// savePage saves the host's standing page on a connected calendar.
func savePage(t *testing.T, f *invitationFixture, enabled bool) crmcontracts.SchedulingProfile {
	t.Helper()
	host := "Test Host"
	profile := defaultSchedulingProfile()
	profile.Provider, profile.Enabled, profile.HostName, profile.BufferMinutes = "gcal", enabled, &host, 0
	saved, err := f.store.SaveSchedulingProfile(f.ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func publicProfileStatus(t *testing.T, f *invitationFixture, slug string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	Handlers{store: f.store}.GetPublicSchedulingProfile(rec, httptest.NewRequest(http.MethodGet, "/profile", nil).WithContext(f.ctx), slug)
	return rec.Code
}

// A page saved before the profile existed names no calendar. The host reads
// it as blocked for want of one, and a guest reads it as an unknown page.
func TestAPageWithNoCalendarIsBlockedForTheHostAndHiddenFromTheGuest(t *testing.T) {
	f := newInvitationFixture(t)
	saved := savePage(t, f, true)
	if _, err := f.env.owner.Exec(f.ctx, `UPDATE booking_page SET scheduling_policy = NULL WHERE host_user_id = $1`, f.env.rep); err != nil {
		t.Fatal(err)
	}
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Blocker == nil || *profile.Blocker != crmcontracts.SchedulingProfileBlockerNoCalendar {
		t.Errorf("blocker = %v, want no_calendar", profile.Blocker)
	}
	if code := publicProfileStatus(t, f, *saved.Slug); code != http.StatusNotFound {
		t.Errorf("a page with no calendar answered its guest %d, want 404", code)
	}
}

// A switched-on page whose calendar cannot take bookings is paused for its
// guest, the same as a page the host switched off.
func TestAPageIsPausedForItsGuestWhenItsCalendarIsBlocked(t *testing.T) {
	f := newInvitationFixture(t)
	saved := savePage(t, f, true)
	if code := publicProfileStatus(t, f, *saved.Slug); code != http.StatusOK {
		t.Fatalf("a live page answered %d", code)
	}
	f.calendar.checkErr = connector.ErrAuthRejected
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Blocker == nil || *profile.Blocker != crmcontracts.SchedulingProfileBlockerCalendarUnavailable {
		t.Errorf("blocker = %v, want calendar_unavailable", profile.Blocker)
	}
	if code := publicProfileStatus(t, f, *saved.Slug); code != http.StatusNotFound {
		t.Errorf("a page with an unusable calendar answered its guest %d, want 404", code)
	}
}

// The second read inside the booking answers a page paused since the first
// one as the first read would: as an unknown page.
func TestTheBookingsSecondReadAnswersAPausedPageAsUnknown(t *testing.T) {
	f := newInvitationFixture(t)
	savePage(t, f, false)
	rec := httptest.NewRecorder()
	start := monday(10)
	req := crmcontracts.BookPublicMeetingJSONRequestBody{Start: start, End: start.Add(30 * time.Minute)}
	_, ok := Handlers{store: f.store}.reservePublicInvitation(rec, httptest.NewRequest(http.MethodPost, "/book", nil).WithContext(f.ctx),
		BookingPage{HostUserID: ids.From[ids.UserKind](f.env.rep)}, req, ids.UUID(f.request.ContactId), invitationIntent{}, MarketingNotAsked)
	if ok || rec.Code != http.StatusNotFound {
		t.Errorf("a page paused before the reservation answered %d, want 404", rec.Code)
	}
}

// A proposal follows its own bookability, not the standing page's switch. It
// stays open on a switched-off page. It pauses only when the calendar cannot
// take the booking it offers.
func TestAProposalPausesOnlyForItsCalendar(t *testing.T) {
	f := newInvitationFixture(t)
	savePage(t, f, true)
	link, err := f.store.CreateProposal(f.ctx, crmcontracts.MeetingProposalRequest{ContactId: f.request.ContactId, AttendeeEmail: f.request.AttendeeEmail, Subject: "Walkthrough", DurationMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Split(link.URL, "proposal-")[1]
	savePage(t, f, false)
	if page := getPublicProposal(f.ctx, t, Handlers{store: f.store}, token); page.Profile.Paused {
		t.Error("a proposal paused with the standing page's switch")
	}
	f.calendar.checkErr = connector.ErrAuthRejected
	if page := getPublicProposal(f.ctx, t, Handlers{store: f.store}, token); !page.Profile.Paused {
		t.Error("a proposal whose calendar cannot take it reads as open")
	}
}
