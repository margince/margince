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
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type existingLengthBooker struct {
	contact ids.UUID
	lead    ids.LeadID
}

func (b existingLengthBooker) EnsureBookerByEmail(context.Context, string, string, string, storekit.ErasureAdmission) (ids.UUID, ids.LeadID, error) {
	return b.contact, b.lead, nil
}

func TestOfferedLengthsReachAvailabilityAndTheActualReservation(t *testing.T) {
	for _, test := range []struct {
		name            string
		requested, want int
		lead            bool
	}{{"offered contact", 45, 45, false}, {"offered lead", 60, 60, true}, {"unknown length defaults", 47, 30, false}} {
		t.Run(test.name, func(t *testing.T) {
			f := newInvitationFixture(t)
			profile := savePage(t, f, true)
			profile.OfferedLengths = new([]int{45, 60})
			profile, err := f.store.SaveSchedulingProfile(f.ctx, profile)
			if err != nil {
				t.Fatal(err)
			}
			booker := existingLengthBooker{contact: ids.UUID(f.request.ContactId)}
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
			saved, err := f.store.SchedulingProfile(f.ctx)
			if err != nil || saved.DurationMinutes != 30 {
				t.Fatalf("host default changed: %+v,%v", saved, err)
			}
		})
	}
}
