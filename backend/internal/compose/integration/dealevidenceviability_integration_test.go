// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The nightly viability check reads a private calendar entry as no word from
// the customer. A dinner does not end the silence, and an accepted forum ahead
// defers nothing. The same dinner left public does end it.

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// newSilentDealWorld is a deal whose buyer last spoke on a call half a year
// ago, judged on the rep's mailbox read three hours ago.
func newSilentDealWorld(t *testing.T) dealEvidenceWorld {
	t.Helper()
	w := newDealEvidenceWorld(t)
	if _, err := OwnerConn(t).Exec(context.Background(), `
		INSERT INTO capture_sync_state (connection_id, last_success_at)
		SELECT id, now() - interval '3 hours' FROM capture_connection WHERE user_id = $1 AND provider = 'gmail'`, w.e.Rep1); err != nil {
		t.Fatal(err)
	}
	subject, at := "Call with the buyer", w.now.AddDate(0, 0, -196)
	if _, _, err := w.e.Activities.LogActivity(w.e.Admin(), activities.LogActivityInput{
		Kind: "call", Subject: &subject, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{
			{EntityType: "deal", EntityID: w.deal.UUID}, {EntityType: "contact", EntityID: w.contact},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return w
}

// viabilityReasons is what the input check reads about the deal now.
func (w dealEvidenceWorld) viabilityReasons(t *testing.T) []viability.Reason {
	t.Helper()
	var subjects []assurance.Subject
	if err := database.WithWorkspaceTx(w.e.Admin(), w.e.Pool, func(tx pgx.Tx) error {
		var err error
		subjects, err = compose.AssuranceSubjects(context.Background(), tx, w.now, []assurance.SourceCoverage{
			{Source: "mail", State: assurance.CoverageChecked, CheckedThrough: &w.now},
		})
		return err
	}); err != nil {
		t.Fatalf("reading the deals to check: %v", err)
	}
	for _, subject := range subjects {
		if subject.DealID == w.deal.String() && subject.Viability != nil {
			if slices.Contains(subject.Viability.Reasons, viability.ReasonCoverageUnknown) {
				t.Fatal("the rep's mailbox reads as unread, so the check observes no silence and no progress")
			}
			return subject.Viability.Reasons
		}
	}
	t.Fatal("the input check did not read the deal")
	return nil
}

// A meeting with the buyer marked private on the rep's calendar is still the
// buyer moving the deal. An accepted one ahead still defers it.
func TestABusinessMeetingMarkedPrivateIsCustomerProgress(t *testing.T) {
	t.Parallel()
	w := newSilentDealWorld(t)
	w.meeting(t, "budget review", w.now.AddDate(0, 0, -2), true)
	if reasons := w.viabilityReasons(t); slices.Contains(reasons, viability.ReasonCustomerSilent) {
		t.Fatalf("the deal with a private-marked review reads %v, want customer progress", reasons)
	}
}

func TestAnAcceptedPrivateMarkedMeetingAheadDefersTheSilentDeal(t *testing.T) {
	t.Parallel()
	w := newSilentDealWorld(t)
	address := "ada-" + ids.NewV7().String() + "@buyer.test"
	buyer, err := w.e.Contacts.CreateContact(w.e.Admin(), contacts.CreateContactInput{
		FullName: "Ada Buyer", Source: provenance.RecordSourceManual,
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	review := CalendarMeeting{
		Event: "review-" + ids.NewV7().String(), At: w.now.AddDate(0, 0, 5), Seat: w.e.Rep1, DeliveredTo: w.calendar, Private: true,
		Links:   []datasource.EntityRef{{Type: datasource.EntityDeal, ID: w.deal.UUID}, {Type: datasource.EntityContact, ID: ids.UUID(buyer.Id)}},
		Parties: []connector.MessageParticipant{{Email: address, Role: "attendee", Response: connector.ResponseAccepted}},
	}
	id := review.Capture(t, w.e)
	if n := w.e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND contact_id = $2 AND response = 'accepted'`,
		id, ids.UUID(buyer.Id)); n != 1 {
		t.Fatalf("the review records %d accepting guests who are the buyer, so nothing here could defer the deal", n)
	}
	if reasons := w.viabilityReasons(t); slices.Contains(reasons, viability.ReasonCustomerSilent) {
		t.Fatalf("the deal with an accepted private-marked review ahead reads %v, want the silence deferred", reasons)
	}
}
