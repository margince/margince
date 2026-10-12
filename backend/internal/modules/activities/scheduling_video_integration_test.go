// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (f *invitationFixture) setVideoDefault(t *testing.T, on *bool) crmcontracts.SchedulingProfile {
	t.Helper()
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	profile.VideoCall = on
	saved, err := f.store.SaveSchedulingProfile(f.ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func (f *invitationFixture) bookAt(t *testing.T, hour int, video *bool) crmcontracts.MeetingInvitation {
	t.Helper()
	f.request.Start, f.request.End, f.request.VideoCall = monday(hour), monday(hour+1), video
	return f.book(t)
}

func TestANewMeetingGetsAVideoCallUnlessTheHostOrTheMeetingSaysNo(t *testing.T) {
	f := newInvitationFixture(t)
	on, off := true, false
	for _, tc := range []struct {
		name      string
		profile   *bool
		requested *bool
		want      bool
	}{
		{"a profile saved before the setting", nil, nil, true},
		{"turned off for one meeting", nil, &off, false},
		{"turned off for the host", &off, nil, false},
		{"asked for despite the host's default", &off, &on, true},
	} {
		f.setVideoDefault(t, tc.profile)
		hour := 9 + len(f.calendar.requests)
		meeting := f.bookAt(t, hour, tc.requested)
		sent := f.calendar.requests[len(f.calendar.requests)-1]
		if sent.VideoCall != tc.want || meeting.VideoCall == nil || *meeting.VideoCall != tc.want {
			t.Fatalf("%s: calendar asked for video = %v, meeting says %v, want %v", tc.name, sent.VideoCall, meeting.VideoCall, tc.want)
		}
		if (meeting.VideoUrl != nil) != tc.want {
			t.Fatalf("%s: video link = %v, want one = %v", tc.name, meeting.VideoUrl, tc.want)
		}
		if meeting.Provider == nil || *meeting.Provider != crmcontracts.MeetingInvitationProviderGcal {
			t.Fatalf("%s: the host's read lost the provider: %+v", tc.name, meeting.Provider)
		}
	}
}

func TestTheGuestSeesTheVideoLinkButNotTheHostsCalendar(t *testing.T) {
	f := newInvitationFixture(t)
	created, err := f.store.CreateInvitation(f.ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	reply := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/public/meeting/token", nil).WithContext(f.ctx)
	Handlers{store: f.store}.GetPublicMeetingInvitation(reply, req, *created.ManagementToken)
	var guest map[string]any
	if err := json.Unmarshal(reply.Body.Bytes(), &guest); err != nil || reply.Code != http.StatusOK {
		t.Fatalf("guest read: %d %s", reply.Code, reply.Body.String())
	}
	if _, leaked := guest["provider"]; leaked {
		t.Fatal("the guest learned which calendar the host uses")
	}
	if _, leaked := guest["calendar_url"]; leaked {
		t.Fatal("the guest received the host's calendar link")
	}
	if link, _ := guest["video_url"].(string); !strings.HasPrefix(link, "https://meet.example.test/") {
		t.Fatalf("the guest lost the join link: %v", guest["video_url"])
	}
}

func TestAProposalsVideoChoiceSurvivesTheGuestsAcceptance(t *testing.T) {
	f := newInvitationFixture(t)
	off := false
	link, err := f.store.CreateProposal(f.ctx, crmcontracts.MeetingProposalRequest{ContactId: f.request.ContactId, AttendeeEmail: f.request.AttendeeEmail, Subject: "Walkthrough", DurationMinutes: 60, VideoCall: &off})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Split(link.URL, "proposal-")[1]
	handlers := Handlers{store: f.store, publicConsent: acceptingConsent{}}

	page := getPublicProposal(f.ctx, t, handlers, token)
	if page.Profile.VideoApp != nil {
		t.Fatalf("the proposal page promised %s for a meeting without video", *page.Profile.VideoApp)
	}

	body := crmcontracts.AcceptPublicMeetingProposalJSONRequestBody{Start: f.request.Start, End: f.request.End}
	body.Consent.Wording, body.Consent.PolicyVersion = "I agree", "v1"
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	reply := httptest.NewRecorder()
	handlers.AcceptPublicMeetingProposal(reply, httptest.NewRequest(http.MethodPost, "/public/proposal", bytes.NewReader(payload)).WithContext(f.ctx), token)
	var accepted crmcontracts.MeetingInvitation
	if err := json.Unmarshal(reply.Body.Bytes(), &accepted); err != nil || reply.Code != http.StatusAccepted {
		t.Fatalf("accept: %d %s", reply.Code, reply.Body.String())
	}
	if accepted.Provider != nil {
		t.Fatal("the accepting guest learned the host's provider")
	}
	meeting, err := f.store.Invitation(f.ctx, ids.UUID(accepted.Id))
	if err != nil {
		t.Fatal(err)
	}
	if meeting.VideoCall == nil || *meeting.VideoCall {
		t.Fatalf("the proposal said no video and the meeting asks for it: %+v", meeting.VideoCall)
	}
}

func TestThePublicPageNamesTheVideoAppOnlyWhenMeetingsGetOne(t *testing.T) {
	f := newInvitationFixture(t)
	handlers := Handlers{store: f.store}
	profile := f.setVideoDefault(t, nil)
	if app := publicVideoApp(f.ctx, t, handlers, *profile.Slug); app != string(crmcontracts.PublicSchedulingProfileVideoAppGoogleMeet) {
		t.Fatalf("video_app = %q, want google_meet by default", app)
	}
	off, on := false, true
	f.setVideoDefault(t, &off)
	if app := publicVideoApp(f.ctx, t, handlers, *profile.Slug); app != "" {
		t.Fatalf("video_app = %q with the host's video turned off", app)
	}
	link, err := f.store.CreateProposal(f.ctx, crmcontracts.MeetingProposalRequest{ContactId: f.request.ContactId, AttendeeEmail: f.request.AttendeeEmail, Subject: "Walkthrough", DurationMinutes: 60, VideoCall: &on})
	if err != nil {
		t.Fatal(err)
	}
	page := getPublicProposal(f.ctx, t, handlers, strings.Split(link.URL, "proposal-")[1])
	if page.Profile.VideoApp == nil || *page.Profile.VideoApp != crmcontracts.PublicSchedulingProfileVideoAppGoogleMeet {
		t.Fatalf("a proposal asking for video did not say which app: %+v", page.Profile.VideoApp)
	}
}

func publicVideoApp(ctx context.Context, t *testing.T, handlers Handlers, slug string) string {
	t.Helper()
	reply := httptest.NewRecorder()
	handlers.GetPublicSchedulingProfile(reply, httptest.NewRequest(http.MethodGet, "/public/scheduling", nil).WithContext(ctx), slug)
	var out crmcontracts.PublicSchedulingProfile
	if err := json.Unmarshal(reply.Body.Bytes(), &out); err != nil || reply.Code != http.StatusOK {
		t.Fatalf("public profile: %d %s", reply.Code, reply.Body.String())
	}
	if out.VideoApp == nil {
		return ""
	}
	return string(*out.VideoApp)
}

func getPublicProposal(ctx context.Context, t *testing.T, handlers Handlers, token string) crmcontracts.PublicMeetingProposal {
	t.Helper()
	reply := httptest.NewRecorder()
	handlers.GetPublicMeetingProposal(reply, httptest.NewRequest(http.MethodGet, "/public/proposal", nil).WithContext(ctx), token)
	var out crmcontracts.PublicMeetingProposal
	if err := json.Unmarshal(reply.Body.Bytes(), &out); err != nil || reply.Code != http.StatusOK {
		t.Fatalf("proposal page: %d %s", reply.Code, reply.Body.String())
	}
	return out
}

// acceptingConsent stands in for the consent module, a true boundary here:
// what is under test is the booking the acceptance makes.
type acceptingConsent struct{}

func (acceptingConsent) ScopedPurpose(context.Context, *ids.UUID) (ids.UUID, error) {
	return ids.NewV7(), nil
}

func (acceptingConsent) ValidateMarketingPurpose(context.Context, ids.UUID) error { return nil }

func (acceptingConsent) CaptureBookingConsent(context.Context, ids.UUID, BookingConsent) (MarketingOutcome, error) {
	return MarketingNotRequested, nil
}

func (acceptingConsent) RecordBookingInquiry(context.Context, ids.UUID, ids.UUID) error { return nil }
