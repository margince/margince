// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// holdingReceiver fails the first POST, holds the second until released, and
// answers every later one at once: the second is an attempt in flight.
type holdingReceiver struct {
	server  *httptest.Server
	count   atomic.Int64
	arrived chan struct{}
	release chan struct{}
}

func newHoldingReceiver(t *testing.T) *holdingReceiver {
	r := &holdingReceiver{arrived: make(chan struct{}), release: make(chan struct{})}
	r.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch r.count.Add(1) {
		case 1:
			w.WriteHeader(http.StatusInternalServerError)
		case 2:
			close(r.arrived)
			<-r.release
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(r.server.Close)
	return r
}

// parkAndHoldARetry parks one delivery as retrying, then starts a sweep whose
// attempt the receiver holds. It returns the subscription and a channel that
// closes when that sweep has finished.
func parkAndHoldARetry(t *testing.T, we *webhookEnv, rcv *holdingReceiver, now *time.Time) (string, <-chan struct{}) {
	t.Helper()
	first := newTestDeliverer(we, now, rcv.server.Client())
	subID, _ := we.createSubscription(t, rcv.server.URL+"/hook", []string{"deal.created"})
	env := makeEnvelope(we.wsID, "deal.created")
	env.Entity.ID = mustParseUUID(t, apptest.CreateOpenDeal(t, we.AppEnv, apptest.DiscoverSeededPipeline(t, we.AppEnv)))
	if err := first.HandleEvent(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	assertDeliveryStatus(t, we, subID, "retrying", 1)
	*now = now.Add(time.Minute)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := first.SweepOnce(context.Background()); err != nil {
			t.Errorf("the holding sweep: %v", err)
		}
	}()
	<-rcv.arrived
	return subID, done
}

func TestTwoSweepsOverOneDueDeliverySendItOnce(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newHoldingReceiver(t)
	now := clocktest.Now(t)
	subID, done := parkAndHoldARetry(t, we, rcv, &now)

	second := newTestDeliverer(we, &now, rcv.server.Client())
	if err := second.SweepOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(rcv.release)
	<-done
	if n := rcv.count.Load(); n != 2 {
		t.Fatalf("receiver saw %d requests, want the first attempt and one retry", n)
	}
	assertDeliveryStatus(t, we, subID, "delivered", 2)
}

func TestAReplayRefusesADeliveryInFlightOrAlreadyDelivered(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newHoldingReceiver(t)
	now := clocktest.Now(t)
	subID, done := parkAndHoldARetry(t, we, rcv, &now)
	path := fmt.Sprintf("/v1/webhook-subscriptions/%s/deliveries/%s/replay", subID, we.deliveryID(t, subID))

	if status := we.Call(t, "POST", path, nil, nil, nil); status != http.StatusConflict {
		t.Errorf("replaying a delivery a sweep is sending → %d, want 409", status)
	}
	close(rcv.release)
	<-done
	assertDeliveryStatus(t, we, subID, "delivered", 2)
	if status := we.Call(t, "POST", path, nil, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("replaying a delivered delivery → %d, want 422", status)
	}
	if n := rcv.count.Load(); n != 2 {
		t.Fatalf("receiver saw %d requests, want 2", n)
	}
}
