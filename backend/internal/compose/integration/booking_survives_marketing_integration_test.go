// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// THE MEETING IS THE THING THE SUBJECT CAME FOR. A newsletter question that
// cannot be asked must not cost them it.
//
// The booking handler records consent and then books. CaptureBookingConsent
// mints the marketing confirmation link as its last act, and any refusal there
// returns from the handler before BookMeeting is ever reached — so a person
// whose primary address was archived, or a purpose archived between the
// operator's form being published and the booking being made, loses the slot.
//
// What the subject sees is a booking form that failed. They try again, get the
// same refusal, and the meeting never happens. Nothing tells them, or the host,
// that the reason was a subscription question they could have simply declined.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// TestABookingSurvivesAMarketingQuestionThatCannotBeAsked is the slice.
//
// The failure is forced through a person carrying no LIVE address. That state
// is ordinary rather than contrived: the ensure resolves an existing person by
// any address they hold, including an archived one, and the mint posts to their
// live primary — which a person whose address was corrected no longer has under
// the old spelling.
func TestABookingSurvivesAMarketingQuestionThatCannotBeAsked(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	base := "/v1/public/booking/" + bookingSlug(t, e)
	transactional := seededTransactionalPurposeID(t, e)
	marketing := seededMarketingPurposeID(t, e)
	monday := nextMonday()

	// A booker whose only address is archived: the ensure will find them, and
	// the mint will have no live mailbox to post the question to.
	const address = "archived@visitor.example"
	var personID string
	if err := e.Owner.QueryRow(context.Background(), `
		INSERT INTO person (full_name, source, captured_by)
		VALUES ('Archie Archived', 'manual', 'human:x')
		RETURNING id`).Scan(&personID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO person_email (person_id, email, is_primary, source, captured_by, archived_at)
		VALUES ($1, $2, true, 'manual', 'human:x', now())`, personID, address); err != nil {
		t.Fatal(err)
	}

	body := AnyMap{
		"start": monday.Add(7 * time.Hour), "end": monday.Add(450 * time.Minute),
		"booker": AnyMap{"name": "Archie Archived", "email": address},
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
		Booking   string `json:"booking"`
		Marketing string `json:"marketing"`
	}
	status := publicCall(t, e, "POST", base, body, nil, &answer)
	if status != http.StatusCreated {
		t.Errorf("a booking whose newsletter question could not be asked → %d, want 201: the "+
			"subject came for a meeting and the tick was optional, so a question that cannot be "+
			"asked must cost them the newsletter and not the slot", status)
	}

	var meetings int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM activity WHERE kind = 'meeting'`).Scan(&meetings); err != nil {
		t.Fatal(err)
	}
	if meetings != 1 {
		t.Errorf("%d meeting(s) booked, want 1 — the slot the subject asked for is gone and "+
			"nothing tells them a subscription question is why", meetings)
	}

	// AND THE BOOKER IS TOLD. They ticked a box; no mail is coming. A response
	// that named only the meeting would leave them waiting for one, and leave
	// the host with no sign that a live form has stopped asking its question.
	if answer.Booking != "confirmed" {
		t.Errorf("the response says booking=%q, want confirmed", answer.Booking)
	}
	if answer.Marketing != "not_asked" {
		t.Errorf("the response says marketing=%q, want not_asked — the subject ticked the box and "+
			"no question could be put, which is a fact they and the host both need", answer.Marketing)
	}

	// The operational grant still stands. It is what the meeting itself rides
	// on, and it was recorded before the question was ever attempted.
	var grants int
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM person_consent
		 WHERE person_id = $1 AND purpose_id = $2 AND state = 'granted'`,
		personID, transactional).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 1 {
		t.Errorf("%d operational grant(s), want 1 — the meeting's own lawful basis went with the "+
			"newsletter question", grants)
	}
}
