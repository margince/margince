// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestAContendedPermitPostponesWithoutRecordingAProviderFailure(t *testing.T) {
	store := &fakeStore{delivery: liveDelivery()}
	sender := &fakeSender{}
	gate := &stubConsent{permitBusy: true}
	d := newTestDispatcher(store, fakeResolver{sender: sender, granted: []string{sendScope}}, gate)
	outcome, err := dispatch(context.Background(), d, store.delivery.ID)
	if err != nil || outcome != OutcomePostponed || sender.calls != 0 || store.failed != "" || store.deferred == "" {
		t.Fatalf("busy permit: outcome=%s err=%v calls=%d failed=%q deferred=%q", outcome, err, sender.calls, store.failed, store.deferred)
	}
}

func TestAnUnhealthyPermitRetriesBeforeCallingTheProvider(t *testing.T) {
	store := &fakeStore{delivery: liveDelivery()}
	sender := &fakeSender{}
	gate := &stubConsent{readyErr: errors.New("lease unavailable")}
	d := newTestDispatcher(store, fakeResolver{sender: sender, granted: []string{sendScope}}, gate)
	outcome, err := dispatch(context.Background(), d, store.delivery.ID)
	if err == nil || outcome != OutcomeRetry || sender.calls != 0 || store.failed == "" || gate.lastPermit == nil || !gate.lastPermit.closed {
		t.Fatalf("unhealthy permit: outcome=%s err=%v calls=%d failed=%q permit=%+v", outcome, err, sender.calls, store.failed, gate.lastPermit)
	}
}

func TestACanceledHandoffHasAnUnknownOutcomeAndReleasesItsPermit(t *testing.T) {
	store := &fakeStore{delivery: liveDelivery()}
	gate := &stubConsent{}
	d := newTestDispatcher(store, sendingResolver(), gate)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticket := commsauthz.TransmitTicket{DeliveryID: store.delivery.ID, Attempt: store.delivery.Attempts, DecisionSetID: ids.NewV7(), Allowed: true}
	_, outcome, _, err := d.authorizedHandoff(ctx, store.delivery, sendSeam{detectsPriorSend: true}, ticket, func(wire context.Context) (connector.SendReceipt, error) {
		if _, bounded := wire.Deadline(); !bounded {
			t.Fatal("provider handoff has no deadline")
		}
		cancel()
		return connector.SendReceipt{}, wire.Err()
	})
	if outcome != outcomeUndecided || !errors.Is(err, connector.ErrSendOutcomeUnknown) || gate.lastPermit == nil || !gate.lastPermit.closed {
		t.Fatalf("canceled handoff: outcome=%s err=%v permit=%+v", outcome, err, gate.lastPermit)
	}
}
