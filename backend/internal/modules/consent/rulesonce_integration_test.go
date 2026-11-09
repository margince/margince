// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// TestStagingRefusesAdvertisingComposedWithoutItsDuties: a body the composer
// built as correspondence, whose recipient the engine reads as marketing at
// staging, is refused. The same body composed with the duties stages.
//
// Mutation: drop the OmitsAdvertisingDuties check in stageDecisions and this
// fails.
func TestStagingRefusesAdvertisingComposedWithoutItsDuties(t *testing.T) {
	e := setupResolve(t)
	for _, tc := range []struct {
		name    string
		omits   bool
		refused bool
	}{
		{"composed without the duties", true, true},
		{"composed with the duties", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			delivery := e.plantDelivery(t)
			err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
				_, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
					Recipients: []connector.Recipient{{Email: e.address}},
					Context:    commsauthz.CategoryMarketing, Subject: "Offer", Body: "body",
					OmitsAdvertisingDuties: tc.omits,
				})
				return err
			})
			if refused := errors.Is(err, errAdvertisingDutiesMissing); refused != tc.refused {
				t.Errorf("staging answered %v, want refused=%v", err, tc.refused)
			}
		})
	}
}

// TestACountryEditMidSendCannotSplitTheDecision edits a recipient's country
// while the send waits on another recipient's cap lock. The recipient had no
// country when the send resolved its countries, so no lock was taken for it.
// The count, the verdict and the recorded stamp must all stay under that one
// answer. Counting under the new, capped country would run the count with no
// lock held for the address. Every recipient's row is checked.
//
// Mutation: re-read the country inside applyFrequencyCap and this fails.
func TestACountryEditMidSendCannotSplitTheDecision(t *testing.T) {
	e := setupCap(t)
	e.seedMarketingSubject(t)
	gate := e.cappedGate(t, 1)
	e.subjectLivesIn(t, "")
	const held = "held@cap.test"
	e.grantMarketingTo(t, held)
	// One advertising message already reached e.address, so under a cap of one
	// a count would refuse the next.
	e.deliveredAdvertising(t, time.Now().Add(-time.Minute))

	holder := holdingSession(t)
	holdCapLock(t, holder, held)
	released := false
	t.Cleanup(func() {
		if !released {
			releaseCapLock(t, holder, held)
		}
	})
	type result struct {
		ticket commsauthz.TransmitTicket
		err    error
	}
	done := make(chan result, 1)
	go func() {
		ticket, err := gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
			DeliveryID: e.decisionDelivery, Attempt: 1,
			Recipients: []connector.Recipient{{Email: held}, {Email: e.address}},
			PurposeKey: marketingPurposeKey, Subject: "Offer", Body: "body",
		})
		done <- result{ticket, err}
	}()
	waitUntilBlockedBy(t, holder)
	e.subjectLivesIn(t, e.capCountry)
	releaseCapLock(t, holder, held)
	released = true
	got := <-done
	if got.err != nil {
		t.Fatalf("authorizing the transmit: %v", got.err)
	}

	rows, err := e.owner.Query(context.Background(), `
		SELECT recipient_address, verdict, ruleset_codes FROM communication_decision
		 WHERE decision_set_id = $1 AND phase = 'transmit'`, got.ticket.DecisionSetID)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Address, Verdict string
		Codes            []string
	}
	all, err := pgx.CollectRows(rows, pgx.RowToStructByPos[row])
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("recorded %d transmit rows, want one per recipient: %+v", len(all), all)
	}
	for _, r := range all {
		switch r.Address {
		case e.address:
			if r.Verdict != string(commsauthz.VerdictAllow) || len(r.Codes) != 0 {
				t.Errorf("%s was judged %s under %v. It was resolved with no country, so no lock "+
					"was taken and no cap may count it", r.Address, r.Verdict, r.Codes)
			}
		case held:
			if r.Verdict != string(commsauthz.VerdictAllow) || len(r.Codes) != 1 || r.Codes[0] != e.capCountry {
				t.Errorf("%s was judged %s under %v, want allow under [%s]", r.Address, r.Verdict, r.Codes, e.capCountry)
			}
		default:
			t.Errorf("a row for an address nobody sent to: %s", r.Address)
		}
	}
}
