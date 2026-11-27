// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

type statedBookingPrivacy struct {
	acceptingConsent
	address string
}

func (p statedBookingPrivacy) BookingPrivacyInformation(context.Context) (string, error) {
	return p.address, nil
}

func TestAllBookingProfileDoorsShowTheSamePrivacyAddress(t *testing.T) {
	f := newInvitationFixture(t)
	profile := savePage(t, f, true)
	address := "https://example.test/privacy"
	handlers := (Handlers{store: f.store}).WithPublicBooking(nil, statedBookingPrivacy{address: address})
	for _, read := range []func(*httptest.ResponseRecorder, *http.Request){
		func(w *httptest.ResponseRecorder, r *http.Request) { handlers.GetSchedulingProfile(w, r) },
		func(w *httptest.ResponseRecorder, r *http.Request) {
			handlers.GetPublicSchedulingProfile(w, r, *profile.Slug, crmcontracts.GetPublicSchedulingProfileParams{})
		},
	} {
		rec := httptest.NewRecorder()
		read(rec, httptest.NewRequest(http.MethodGet, "/profile", nil).WithContext(f.ctx))
		var got struct {
			PrivacyInformation string `json:"privacy_information"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || got.PrivacyInformation != address {
			t.Fatalf("profile: %d %s", rec.Code, rec.Body.String())
		}
	}
	proposal, err := f.store.CreateProposal(f.ctx, crmcontracts.MeetingProposalRequest{ContactId: f.request.ContactId, AttendeeEmail: f.request.AttendeeEmail, Subject: "Walkthrough", DurationMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	page := getPublicProposal(f.ctx, t, handlers, strings.Split(proposal.URL, "proposal-")[1])
	if page.Profile.PrivacyInformation == nil || *page.Profile.PrivacyInformation != address {
		t.Fatalf("proposal privacy: %+v", page.Profile.PrivacyInformation)
	}
}
