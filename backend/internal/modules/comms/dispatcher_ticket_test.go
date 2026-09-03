// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// What stands between a delivery and a provider: a decision recorded for THIS
// delivery and THIS attempt.
//
// The check exists because a send that reaches a provider without one is a
// send nobody can account for afterwards — the installation cannot say what
// rule permitted it or what evidence stood behind it. These tests hold the two
// ways that could happen quietly: no decision at all, and a decision belonging
// to an earlier attempt whose world may already have moved.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// No ticket, no wire. A zero decision-set id means the engine recorded nothing.
func TestTransmitRefusesWhenNoDecisionWasRecorded(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{delivery: liveDelivery()}
	// Everything present EXCEPT the decision-set id, so this test fails on the
	// one thing it is about: nothing was recorded.
	consent := &stubConsent{armed: true, ticket: commsauthz.TransmitTicket{
		DeliveryID: store.delivery.ID,
		Attempt:    store.delivery.Attempts,
		Allowed:    true,
	}}
	d := newTestDispatcher(store, fakeResolver{sender: sender, granted: []string{sendScope}}, consent)

	got, err := dispatch(context.Background(), d, store.delivery.ID)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got != OutcomeParked {
		t.Errorf("outcome = %v, want OutcomeParked — an unrecorded send must not reach a provider", got)
	}
	if sender.calls != 0 {
		t.Errorf("provider called %d time(s) with no decision on record", sender.calls)
	}
}

// A ticket for a DIFFERENT attempt is not a ticket for this one. The delivery
// it names was authorized against a world that may since have changed — a
// withdrawal, an objection, a bounce — which is the whole reason the recheck
// runs per attempt rather than once.
func TestTransmitRefusesATicketFromAnotherAttempt(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{delivery: liveDelivery()}
	consent := &stubConsent{armed: true, ticket: commsauthz.TransmitTicket{
		DeliveryID:    store.delivery.ID,
		DecisionSetID: ids.NewV7(),
		// Any attempt but the one being dispatched.
		Attempt: store.delivery.Attempts + 41,
		Allowed: true,
	}}
	d := newTestDispatcher(store, fakeResolver{sender: sender, granted: []string{sendScope}}, consent)

	got, err := dispatch(context.Background(), d, store.delivery.ID)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got != OutcomeParked {
		t.Errorf("outcome = %v, want OutcomeParked — a stale decision authorizes nothing", got)
	}
	if sender.calls != 0 {
		t.Errorf("provider called %d time(s) on a stale decision", sender.calls)
	}
}

// The engine refusing is an ANSWER and parks. This is the observe-mode path
// that must still hold: the four absolute refusals deny whatever the legacy
// gate says.
func TestTransmitParksWhenTheEngineRefuses(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{delivery: liveDelivery()}
	consent := &stubConsent{armed: true, ticket: commsauthz.TransmitTicket{
		DecisionSetID: ids.NewV7(),
		Allowed:       false,
		Reason:        "1 of 1 recipients are not authorized for this message (marketing_objection)",
	}}
	d := newTestDispatcher(store, fakeResolver{sender: sender, granted: []string{sendScope}}, consent)

	got, err := dispatch(context.Background(), d, store.delivery.ID)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got != OutcomeParked || sender.calls != 0 {
		t.Errorf("outcome=%v calls=%d, want parked/0 on an engine refusal", got, sender.calls)
	}
}

// An engine that could not answer is an OUTAGE, not a verdict. Reading it as a
// refusal would park legitimate mail permanently every time the database
// hiccuped — the same mistake the legacy gate's own branch guards against.
func TestTransmitRetriesWhenTheEngineCannotAnswer(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{delivery: liveDelivery()}
	consent := &stubConsent{authzErr: errors.New("connection refused")}
	d := newTestDispatcher(store, fakeResolver{sender: sender, granted: []string{sendScope}}, consent)

	got, err := dispatch(context.Background(), d, store.delivery.ID)
	if err == nil && got != OutcomeRetry {
		t.Errorf("outcome = %v, want OutcomeRetry — an outage is not a refusal", got)
	}
	if sender.calls != 0 {
		t.Errorf("provider called %d time(s) while the engine was unreachable", sender.calls)
	}
}

// And the converse, or every test above would pass with the wire disconnected:
// an ordinary delivery with a current ticket does reach the provider.
func TestTransmitProceedsOnACurrentTicket(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{delivery: liveDelivery()}
	consent := &stubConsent{}
	d := newTestDispatcher(store, fakeResolver{sender: sender, granted: []string{sendScope}}, consent)

	got, err := dispatch(context.Background(), d, store.delivery.ID)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got != OutcomeSent || sender.calls != 1 {
		t.Errorf("outcome=%v calls=%d, want sent/1 on a current decision", got, sender.calls)
	}
	if consent.authzSeen != 1 {
		t.Errorf("the engine was asked %d time(s), want exactly 1 per attempt", consent.authzSeen)
	}
}
