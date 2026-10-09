// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

// A receiver that cannot be reached is re-attempted on the declared schedule
// and then dead-lettered. The sweep runs the way the job worker runs it, with
// the queue's bare context: no workspace bound and no actor. The subject is a
// real deal created through the API, owned by the subscription owner.

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/jobtest"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/database"
)

// unresolvableTarget is a name under the reserved .invalid domain, which no
// resolver answers.
const unresolvableTarget = "https://example-retry.invalid/hook"

// unresolvableClient fails every request the way a lookup of
// unresolvableTarget fails, without asking a real resolver.
func unresolvableClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, &net.DNSError{Err: "no such host", Name: req.URL.Hostname(), IsNotFound: true}
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// parkDealDelivery subscribes to deal.created at the unresolvable target,
// creates a deal through the API and drives the first attempt, which fails.
func parkDealDelivery(t *testing.T, we *webhookEnv, deliverer *webhooks.Deliverer) string {
	t.Helper()
	subID, _ := we.createSubscription(t, unresolvableTarget, []string{"deal.created"})
	dealID := apptest.CreateOpenDeal(t, we.AppEnv, apptest.DiscoverSeededPipeline(t, we.AppEnv))
	env := makeEnvelope(we.wsID, "deal.created")
	env.Entity.ID = mustParseUUID(t, dealID)
	if err := deliverer.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("the first delivery attempt: %v", err)
	}
	assertDeliveryStatus(t, we, subID, "retrying", 1)
	return subID
}

func TestTheRetryJobReattemptsADueDelivery(t *testing.T) {
	we := setupWebhooks(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	subID := parkDealDelivery(t, we, newTestDeliverer(we, &now, unresolvableClient()))
	now = now.Add(64 * time.Second)

	_, completed, failed := jobtest.StartTestJobRunner(t, we.pool, compose.JobRunnerConfig{
		WebhookRetry: compose.WebhookRetryConfig{
			Deliverer: func(db *database.DB) *webhooks.Deliverer {
				return newTestDelivererOn(we, db, &now, unresolvableClient())
			},
		},
	})
	waitCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if !jobtest.AwaitKindOutcome(waitCtx, t, completed, failed, compose.WebhookRetryArgs{}.Kind()) {
		t.Fatal("the retry pass failed")
	}
	assertDeliveryStatus(t, we, subID, "retrying", 2)
	// A pass that could not re-check the owner's access also spends an
	// attempt, so the count alone does not prove the request went out.
	if reason := we.deliveryError(t, subID); !strings.Contains(reason, "request failed") {
		t.Errorf("the re-attempted row says %q, want the failed request", reason)
	}
}

func TestAnUnreachableReceiverIsDeadLetteredOnSchedule(t *testing.T) {
	we := setupWebhooks(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	deliverer := newTestDeliverer(we, &now, unresolvableClient())
	subID := parkDealDelivery(t, we, deliverer)

	for attempts, gap := 1, time.Second; attempts < 6; attempts, gap = attempts+1, gap*2 {
		// A pass just short of the deadline must leave the row alone, so the
		// re-attempt below is the schedule's doing and not any pass's.
		now = now.Add(gap - time.Millisecond)
		if err := deliverer.SweepOnce(context.Background()); err != nil {
			t.Fatalf("sweep before the deadline: %v", err)
		}
		assertDeliveryStatus(t, we, subID, "retrying", attempts)

		now = now.Add(time.Millisecond)
		if err := deliverer.SweepOnce(context.Background()); err != nil {
			t.Fatalf("sweep at the deadline: %v", err)
		}
		if attempts < 5 {
			assertDeliveryStatus(t, we, subID, "retrying", attempts+1)
		}
	}
	assertDeliveryStatus(t, we, subID, "dead_lettered", 6)
	if reason := we.deliveryError(t, subID); !strings.Contains(reason, "request failed") {
		t.Errorf("the dead-lettered row says %q, want the failed request", reason)
	}
}

func TestADeliveryWhoseAccessCannotBeRecheckedEndsDeadLetteredWithItsReason(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusInternalServerError)
	now := time.Now().UTC().Truncate(time.Microsecond)
	subID, _ := we.createSubscription(t, rcv.server.URL+"/hook", []string{"deal.created"})
	enqueuer := newTestDeliverer(we, &now, rcv.server.Client())
	if err := enqueuer.HandleEvent(context.Background(), makeEnvelope(we.wsID, "deal.created")); err != nil {
		t.Fatalf("the first delivery attempt: %v", err)
	}
	sweeper := newTestDelivererWithResolver(we, &now, rcv.server.Client(),
		failingResolver{err: errors.New("identity store unavailable")})

	for range 8 {
		now = now.Add(64 * time.Second)
		if err := sweeper.SweepOnce(context.Background()); err != nil {
			t.Fatalf("sweep: %v", err)
		}
	}
	assertDeliveryStatus(t, we, subID, "dead_lettered", 6)
	if got := rcv.count.Load(); got != 1 {
		t.Errorf("the endpoint saw %d attempts, want only the enqueue attempt: an unchecked delivery must not be sent", got)
	}
	if reason := we.deliveryError(t, subID); !strings.Contains(reason, "could not be re-checked") {
		t.Errorf("the dead-lettered row says %q, want the failed re-check", reason)
	}
}
