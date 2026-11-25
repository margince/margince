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
	"slices"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type existingLengthBooker struct {
	contact ids.UUID
	lead    ids.LeadID
	created bool
	// host is the booking page host the handler named, and fills counts the
	// gap fills it asked for; the test reads both back.
	host  *ids.UserID
	fills *int
}

func (b existingLengthBooker) EnsureBookerByEmail(_ context.Context, host ids.UserID, _, _, _ string, _ storekit.ErasureAdmission) (ids.UUID, ids.LeadID, bool, error) {
	*b.host = host
	return b.contact, b.lead, b.created, nil
}

func (b existingLengthBooker) FillBookerGaps(context.Context, ids.UUID, ids.UserID, string) error {
	*b.fills++
	return nil
}

func TestOfferedLengthsReachAvailabilityAndTheActualReservation(t *testing.T) {
	for _, test := range []struct {
		name            string
		requested, want int
		lead, created   bool
	}{{"offered contact", 45, 45, false, false}, {"offered lead", 60, 60, true, false}, {"created contact", 45, 45, false, true}} {
		t.Run(test.name, func(t *testing.T) {
			f := newInvitationFixture(t)
			profile := savePage(t, f, true)
			profile.OfferedLengths = new([]int{45, 60})
			profile, err := f.store.SaveSchedulingProfile(f.ctx, profile)
			if err != nil {
				t.Fatal(err)
			}
			booker := existingLengthBooker{contact: ids.UUID(f.request.ContactId), created: test.created, host: new(ids.UserID), fills: new(int)}
			if test.lead {
				booker.contact = ids.Nil
				booker.lead = ids.New[ids.LeadKind]()
				if _, err := f.env.owner.Exec(f.ctx, `INSERT INTO lead(id,full_name,email,source,captured_by) VALUES($1,'Guest','guest@example.test','manual','human:test')`, booker.lead); err != nil {
					t.Fatal(err)
				}
			}
			handler := (Handlers{store: f.store}).WithPublicBooking(booker, acceptingConsent{})
			available := httptest.NewRecorder()
			handler.GetPublicAvailability(available, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(f.ctx), *profile.Slug, crmcontracts.GetPublicAvailabilityParams{From: monday(9), To: monday(17), L: &test.requested})
			var slots struct {
				Slots []slot `json:"slots"`
			}
			if err := json.Unmarshal(available.Body.Bytes(), &slots); err != nil || available.Code != http.StatusOK || len(slots.Slots) == 0 {
				t.Fatalf("availability: %d %s", available.Code, available.Body.String())
			}
			slot := slots.Slots[0]
			if slot.End.Sub(slot.Start) != time.Duration(test.want)*time.Minute {
				t.Fatalf("slot length=%v", slot.End.Sub(slot.Start))
			}
			body := crmcontracts.BookPublicMeetingJSONRequestBody{Start: slot.Start, End: slot.End}
			body.Booker.Name, body.Booker.Email = "Guest", "guest@example.test"
			body.Consent.PolicyVersion, body.Consent.Wording = "2026-07", new("I agree to booking contact")
			payload, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/book", bytes.NewReader(payload)).WithContext(f.ctx)
			request.Header.Set("Idempotency-Key", "2943cb96-b3c5-4445-8685-20d6fe8bb026")
			response := httptest.NewRecorder()
			handler.BookPublicMeeting(response, request, *profile.Slug, crmcontracts.BookPublicMeetingParams{L: &test.requested})
			if response.Code != http.StatusCreated {
				t.Fatalf("reservation: %d %s", response.Code, response.Body.String())
			}
			if booker.host.UUID != f.env.rep {
				t.Errorf("the booker was ensured for host %s, want the page's host %s", booker.host, f.env.rep)
			}
			// Only an existing contact is filled: a created one already has it all.
			if want := map[bool]int{true: 0, false: 1}[test.lead || test.created]; *booker.fills != want {
				t.Errorf("gap fills = %d, want %d", *booker.fills, want)
			}
			saved, err := f.store.SchedulingProfile(f.ctx)
			if err != nil || saved.DurationMinutes != 30 {
				t.Fatalf("host default changed: %+v,%v", saved, err)
			}
		})
	}
}

func TestTheMainLinkOffersEveryLengthAndAPausedOrProposalPageNone(t *testing.T) {
	f := newInvitationFixture(t)
	handlers := Handlers{store: f.store}
	profile := savePage(t, f, true)
	profile.OfferedLengths = new([]int{45, 30, 15})
	profile, err := f.store.SaveSchedulingProfile(f.ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, requested := range []*int{nil, new(45), new(47)} {
		reply := httptest.NewRecorder()
		handlers.GetPublicSchedulingProfile(reply, httptest.NewRequest(http.MethodGet, "/profile", nil).WithContext(f.ctx), *profile.Slug, crmcontracts.GetPublicSchedulingProfileParams{L: requested})
		var guest crmcontracts.PublicSchedulingProfile
		if err := json.Unmarshal(reply.Body.Bytes(), &guest); err != nil || guest.OfferedLengths == nil || !slices.Equal(*guest.OfferedLengths, []int{30, 45, 15}) {
			t.Fatalf("l=%v: %d %s, want lengths [30 45 15]", requested, reply.Code, reply.Body.String())
		}
	}
	link, err := f.store.CreateProposal(f.ctx, crmcontracts.MeetingProposalRequest{ContactId: f.request.ContactId, AttendeeEmail: f.request.AttendeeEmail, Subject: "Walkthrough", DurationMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	if proposal := getPublicProposal(f.ctx, t, handlers, strings.Split(link.URL, "proposal-")[1]); proposal.Profile.OfferedLengths != nil {
		t.Fatalf("a proposal offered the page's lengths %v", *proposal.Profile.OfferedLengths)
	}
	profile.Enabled = false
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	assertPausedForGuest(t, guestProfile(t, f, *profile.Slug))
}

func TestALengthRemovedAfterThePageLoadedIsRefusedAndNeverBookedAsTheDefault(t *testing.T) {
	f := newInvitationFixture(t)
	profile := savePage(t, f, true)
	profile.OfferedLengths = new([]int{45})
	profile, err := f.store.SaveSchedulingProfile(f.ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	handler := (Handlers{store: f.store}).WithPublicBooking(existingLengthBooker{contact: ids.UUID(f.request.ContactId), host: new(ids.UserID), fills: new(int)}, acceptingConsent{})
	shown := httptest.NewRecorder()
	handler.GetPublicAvailability(shown, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(f.ctx), *profile.Slug, crmcontracts.GetPublicAvailabilityParams{From: monday(9), To: monday(17), L: new(45)})
	var slots struct {
		Slots []slot `json:"slots"`
	}
	if err := json.Unmarshal(shown.Body.Bytes(), &slots); err != nil || len(slots.Slots) == 0 {
		t.Fatalf("availability: %d %s", shown.Code, shown.Body.String())
	}
	profile.OfferedLengths = new([]int{})
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	meetingsBefore := countActivities(t, f)
	// The guest still holds a 45-minute time; the default, 30, starts at the same minute.
	start := slots.Slots[0].Start
	for key, end := range map[string]time.Time{"7c0d5e0a-3b1f-4c55-9d2e-6a1b8f4e2c10": start.Add(45 * time.Minute), "b3e1a9d4-58c2-4f0e-a7b6-2d9c4e1f8a37": start.Add(30 * time.Minute)} {
		body := crmcontracts.BookPublicMeetingJSONRequestBody{Start: start, End: end}
		body.Booker.Name, body.Booker.Email = "Guest", "guest@example.test"
		body.Consent.PolicyVersion, body.Consent.Wording = "2026-07", new("I agree to booking contact")
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/book", bytes.NewReader(payload)).WithContext(f.ctx)
		request.Header.Set("Idempotency-Key", key)
		booked := httptest.NewRecorder()
		handler.BookPublicMeeting(booked, request, *profile.Slug, crmcontracts.BookPublicMeetingParams{L: new(45)})
		if booked.Code != http.StatusUnprocessableEntity || !strings.Contains(booked.Body.String(), "length_not_offered") {
			t.Fatalf("booking %v under a removed 45-minute length: %d %s", end.Sub(start), booked.Code, booked.Body.String())
		}
	}
	times := httptest.NewRecorder()
	handler.GetPublicAvailability(times, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(f.ctx), *profile.Slug, crmcontracts.GetPublicAvailabilityParams{From: monday(9), To: monday(17), L: new(45)})
	if times.Code != http.StatusUnprocessableEntity || !strings.Contains(times.Body.String(), "length_not_offered") {
		t.Fatalf("times for a removed length: %d %s", times.Code, times.Body.String())
	}
	if after := countActivities(t, f); after != meetingsBefore {
		t.Fatalf("a refused length booked a meeting: %d activities, was %d", after, meetingsBefore)
	}
}

func countActivities(t *testing.T, f *invitationFixture) int {
	t.Helper()
	var count int
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM activity`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
