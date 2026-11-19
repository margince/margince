// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// With the mail lane wired, a marketing tick on the booking form mails the
// confirmation link by itself, once. It grants nothing until the subject
// spends the link.
func TestAMarketingTickMailsOneConfirmationAndARepeatDoesNotMailAgain(t *testing.T) {
	e, _ := setupBookingProvider(t, compose.WithOperatorMail(apptest.DiscardingMailer{}))
	e.BootstrapWorkspace(t)
	apptest.EnableGoogleBookingPage(t, e)
	base := "/v1/public/booking/" + bookingSlug(t, e)
	transactional := seededTransactionalPurposeID(t, e)
	marketing := seededMarketingPurposeID(t, e)
	monday := nextMonday()

	book := func(at time.Duration) string {
		body := AnyMap{
			"start": monday.Add(at), "end": monday.Add(at + 30*time.Minute),
			"booker": AnyMap{"name": "Mia Mailed", "email": "mia@visitor.example"},
			"consent": AnyMap{
				"purpose_id": transactional, "policy_version": "pp-2026-01",
				"wording": "You agree we may contact you about this meeting.",
				"marketing": AnyMap{
					"purpose_id": marketing, "policy_version": "mk-2026-01",
					"wording": "Send me your newsletter.",
				},
			},
		}
		var answer struct {
			Marketing string `json:"marketing"`
		}
		if status := publicCall(t, e, "POST", base, body, nil, &answer); status != http.StatusCreated {
			t.Fatalf("a booking carrying a marketing tick → %d, want 201", status)
		}
		return answer.Marketing
	}
	controllerMails := func() int {
		t.Helper()
		var n int
		if err := e.Owner.QueryRow(context.Background(),
			`SELECT count(*) FROM comms_outbound WHERE sender_kind = 'controller'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if got := book(2 * time.Hour); got != "pending_confirmation" {
		t.Fatalf("first tick reports marketing=%q, want pending_confirmation", got)
	}
	if n := controllerMails(); n != 1 {
		t.Fatalf("first tick staged %d confirmation mails, want exactly 1", n)
	}
	contactID := contactIDByEmail(t, e, "mia@visitor.example")
	assertNoMarketingGrant(t, e, contactID, marketing)

	var read struct {
		State []struct {
			PurposeID    string     `json:"purpose_id"`
			State        string     `json:"state"`
			PendingUntil *time.Time `json:"confirmation_pending_until"`
		} `json:"state"`
	}
	if status := e.Call(t, "GET", "/v1/contacts/"+contactID+"/consent", nil, nil, &read); status != http.StatusOK {
		t.Fatalf("reading the consent state → %d", status)
	}
	for _, st := range read.State {
		if st.PurposeID == marketing && (st.State != "unknown" || st.PendingUntil == nil) {
			t.Errorf("the marketing row reads state=%q pending_until=%v, want unknown with a pending expiry", st.State, st.PendingUntil)
		}
	}

	if got := book(3 * time.Hour); got != "pending_confirmation" {
		t.Fatalf("repeat tick reports marketing=%q, want pending_confirmation", got)
	}
	if n := controllerMails(); n != 1 {
		t.Fatalf("a repeat tick inside the window left %d confirmation mails, want still 1", n)
	}
	assertNoMarketingGrant(t, e, contactID, marketing)
}
