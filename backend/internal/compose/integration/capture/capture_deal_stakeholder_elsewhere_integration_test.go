// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A stakeholder's mail reaches the deal they sit on even when they work at
// another company. Mail arrives through the production sink.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// endSeat ends every seat the contact holds on a deal.
func endSeat(t *testing.T, e *integration.SearchEnv, contact ids.UUID) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE relationship SET ended_at = now() - interval '1 day'
			 WHERE kind = 'deal_stakeholder' AND contact_id = $1`, contact)
		return err
	})
	if err != nil {
		t.Fatalf("ending the seat: %v", err)
	}
}

// A consultant works at a firm with no deal and sits on the committee of a
// client's deal. Their mail goes on that deal.
func TestCaptureFilesAStakeholdersMailOnADealAtAnotherCompany(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	consultancy := w.company(t, "Consultancy")
	carol := w.employee(t, "carol@consultancy.example", consultancy)
	deal := w.deal(t, "Acme rollout")
	w.stakeholder(t, carol, deal)

	env.syncSent(t, map[string]bool{"cx-out@myco.example": true},
		authenticated(mailFrom("carol@consultancy.example", "cx-in@consultancy.example", "", "rollout plan"), "consultancy.example"),
		email(captureOwner, "", "carol@consultancy.example", "cx-out@myco.example", "cx-in@consultancy.example"),
	)

	wantDeals(t, e, "cx-in@consultancy.example", deal)
	wantDeals(t, e, "cx-out@myco.example", deal)
	if n, _ := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("%d deal questions for a single seat, want none", n)
	}
}

// Seats on two open deals at two companies are a question, offering both.
func TestCaptureAsksWhenAStakeholderHoldsSeatsAtTwoCompanies(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	carol := w.employee(t, "carol@consultancy.example", w.company(t, "Consultancy"))
	acme := w.deal(t, "Acme rollout")
	beta := w.at(t, "Beta").deal(t, "Beta rollout")
	w.stakeholder(t, carol, acme)
	w.stakeholder(t, carol, beta)

	env.syncSent(t, map[string]bool{"cx2@myco.example": true},
		email(captureOwner, "", "carol@consultancy.example", "cx2@myco.example", ""))

	wantDeals(t, e, "cx2@myco.example")
	if n, _ := whichDealQuestions(t, e); n != 1 {
		t.Fatalf("%d deal questions for two seats, want exactly 1", n)
	}
	if n := countRows(t, e, `SELECT count(*) FROM activity
		 WHERE source_system = 'which_deal' AND jsonb_array_length(raw->'offered_deal_ids') = 2`); n != 1 {
		t.Fatalf("%d questions offer both deals, want 1", n)
	}
}

// An ended seat, a contact with no seat and the mail's own company deciding
// each leave the other company's deal alone.
func TestCaptureFilesNothingOnAnotherCompanyWithoutALiveSeat(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	consultancy := w.company(t, "Consultancy")
	carol := w.employee(t, "carol@consultancy.example", consultancy)
	w.employee(t, "dave@consultancy.example", consultancy)
	deal := w.deal(t, "Acme rollout")
	w.stakeholder(t, carol, deal)
	endSeat(t, e, carol)

	env.sync(t, mailFrom("carol@consultancy.example", "cx3@consultancy.example", "", "rollout plan"))
	env.sync(t, mailFrom("dave@consultancy.example", "cx4@consultancy.example", "", "rollout plan"))
	wantDeals(t, e, "cx3@consultancy.example")
	wantDeals(t, e, "cx4@consultancy.example")
	if n, _ := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("%d deal questions without a live seat, want none", n)
	}
}

// The mail's own company still wins: its one open deal takes the mail even
// when the sender sits on another company's deal.
func TestCaptureKeepsTheMailsCompanyAheadOfASeatElsewhere(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	alice := w.employee(t, "alice@acme.example", w.customer)
	own := w.deal(t, "Acme rollout")
	elsewhere := w.at(t, "Beta").deal(t, "Beta rollout")
	w.stakeholder(t, alice, elsewhere)

	env.sync(t, mailFrom("alice@acme.example", "cx5@acme.example", "", "status"))
	wantDeals(t, e, "cx5@acme.example", own)
}

// A seat named only in a header is no proof. An unauthenticated mail from the
// stakeholder, and an authenticated stranger's mail that copies them, file
// nothing. Nobody is asked either, because an inbound mail names no colleague
// the sender could not have typed.
func TestCaptureFilesNothingOnAHeaderNamedSeat(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	carol := w.employee(t, "carol@consultancy.example", w.company(t, "Consultancy"))
	deal := w.deal(t, "Acme rollout")
	w.stakeholder(t, carol, deal)

	env.sync(t, mailFrom("carol@consultancy.example", "cx6@consultancy.example", "", "rollout plan"))
	env.sync(t, authenticated(mailFrom("eve@stranger.example", "cx7@stranger.example", "", "rollout plan",
		"carol@consultancy.example"), "stranger.example"))

	wantDeals(t, e, "cx6@consultancy.example")
	wantDeals(t, e, "cx7@stranger.example")
	if n, _ := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("%d deal questions for unproven mail, want none", n)
	}
}
