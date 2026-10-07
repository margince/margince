// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

// The outbound-webhook retry sweep is one job row per pass. A pass whose
// due-retry scan fails must say so: a sweep that reported success is
// indistinguishable from a sweep that found nothing due, and what it hides is
// parked deliveries never being re-attempted at all — the subscriber simply
// stops receiving, with no failed row anywhere to read.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/jobtest"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// webhookSweepCtx is the scope the retry workspace worker binds before it calls
// the engine: the tenant, and nothing else. The sweep resolves no principal of
// its own and writes no audited row — a suite that bound more would be
// exercising a pass production never runs.
//
// It does re-check each delivery's visibility before re-sending, from the
// subscription owner recorded on the row rather than from anything in this
// context (webhookrevisibility_integration_test.go). That is why binding only
// the tenant is still enough.
func webhookSweepCtx(ws ids.UUID) context.Context {
	return principal.WithWorkspaceID(context.Background(), ws)
}

// failDueScans makes reading a webhook_delivery row raise, so the sweep's due
// SCAN fails. There must therefore be a delivery the scan would return: the
// policy is evaluated as the scan reads rows, so a sweep with nothing due runs
// cleanly even while armed.
//
// A RESTRICTIVE row-level policy is the fault seam, not the trigger the other
// fan-out suites use: the only failure this pass can suffer is its due SCAN
// failing, and no trigger fires on a SELECT.
//
// It is dropped in cleanup — the integration lane resets rows between tests but
// keeps the schema, so a surviving policy would blind every later suite that
// reads a delivery.
func failDueScans(t *testing.T, owner *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	if _, err := owner.Exec(ctx, `
		CREATE OR REPLACE FUNCTION webhook_due_scan_fault() RETURNS boolean
		LANGUAGE plpgsql AS $$
		BEGIN
		  RAISE EXCEPTION 'webhook due-scan fault injection';
		END $$`); err != nil {
		t.Fatalf("creating the fault-injection function: %v", err)
	}
	// Registered before the policy is armed, not after both: a failure to arm
	// would otherwise leave the function behind, which is the leak this
	// helper's whole cleanup exists to prevent. Cleanups run LIFO, so the
	// policy below still drops first.
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(),
			`DROP FUNCTION webhook_due_scan_fault()`); err != nil {
			t.Errorf("dropping the fault-injection function: %v", err)
		}
	})
	// The injector arms row security ITSELF, for this table and this test only:
	// a fault injector owning its mechanism keeps this test about the sweep.
	// ENABLE without FORCE is deliberate: the app role the sweep runs as is not
	// this table's owner, so the policy binds it, while the owner connection
	// this fixture seeds through stays unaffected.
	if _, err := owner.Exec(ctx, `ALTER TABLE webhook_delivery ENABLE ROW LEVEL SECURITY`); err != nil {
		t.Fatalf("arming row security for the fault injection: %v", err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(),
			`ALTER TABLE webhook_delivery DISABLE ROW LEVEL SECURITY`); err != nil {
			t.Errorf("disarming row security after the fault injection: %v", err)
		}
	})
	// A permissive policy admitting everything, because a RESTRICTIVE one only
	// narrows what some permissive policy already admitted: with row security
	// on and nothing permissive present the default is deny-all, the scan reads
	// zero rows, and the fault below never runs — the sweep would report a
	// clean pass and this test would fail for the opposite reason.
	if _, err := owner.Exec(ctx, `
		CREATE POLICY webhook_delivery_scan_all ON webhook_delivery USING (true)`); err != nil {
		t.Fatalf("arming the permissive base policy: %v", err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(),
			`DROP POLICY webhook_delivery_scan_all ON webhook_delivery`); err != nil {
			t.Errorf("dropping the permissive base policy: %v", err)
		}
	})
	if _, err := owner.Exec(ctx, `
		CREATE POLICY webhook_delivery_scan_fault ON webhook_delivery
		AS RESTRICTIVE FOR SELECT
		USING (webhook_due_scan_fault())`); err != nil {
		t.Fatalf("arming the fault-injection policy: %v", err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(),
			`DROP POLICY webhook_delivery_scan_fault ON webhook_delivery`); err != nil {
			t.Errorf("dropping the fault-injection policy: %v", err)
		}
	})
}

// parkOneDelivery registers a subscription against a receiver that is down and
// drives one failed attempt, leaving exactly one delivery parked for retry.
func parkOneDelivery(t *testing.T, we *webhookEnv, deliverer *webhooks.Deliverer, rcv *receiver) {
	t.Helper()
	subID, _ := we.createSubscription(t, rcv.server.URL+"/hook", []string{"deal.created"})
	if err := deliverer.HandleEvent(context.Background(), makeEnvelope(we.wsID, "deal.created")); err != nil {
		t.Fatalf("the first delivery attempt: %v", err)
	}
	assertDeliveryStatus(t, we, subID, "retrying", 1)
}

func TestWebhookRetryReportsASweepWhoseDueScanFailed(t *testing.T) {
	we := setupWebhooks(t)
	owner := integration.OwnerConn(t)

	rcv := newReceiver(t, http.StatusInternalServerError) // endpoint is down
	now := time.Now().UTC()
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())
	parkOneDelivery(t, we, deliverer, rcv)

	// One healthy pass first: it proves the parked delivery is reachable by a
	// sweep at this clock reading, so the "not re-attempted" assertion below
	// says the fault held rather than that there was nothing to attempt.
	now = now.Add(64 * time.Second) // beyond the largest backoff gap
	if err := deliverer.SweepOnce(webhookSweepCtx(we.wsID)); err != nil {
		t.Fatalf("the sweep before any fault was injected: %v", err)
	}
	if got := rcv.count.Load(); got != 2 {
		t.Fatalf("the endpoint saw %d attempts, want the enqueue attempt plus one re-attempt — the sweep never reached the parked delivery", got)
	}

	failDueScans(t, owner)
	now = now.Add(64 * time.Second)

	err := deliverer.SweepOnce(webhookSweepCtx(we.wsID))
	if got := rcv.count.Load(); got != 2 {
		t.Fatalf("the fault injection did not hold: the parked delivery was re-attempted %d more times, so the due scan never failed", got-2)
	}
	if err == nil {
		t.Fatal("a sweep whose due-retry scan failed reported success — every parked delivery stays parked and nothing records that the scan never ran")
	}
	// The failure is the one this test injected, not any failure. Without this
	// the assertion above passes on an unrelated break — including the one it is
	// least able to notice, a fault that stopped reaching the scan while
	// something else failed in its place.
	if !strings.Contains(err.Error(), "webhook due-scan fault injection") {
		t.Errorf("the sweep failed with %v, which does not name the injected due-scan fault", err)
	}
}

// TestWebhookRetryRecordsAFailedPassAsAFailedRow is what survives the fan-out.
//
// It used to enqueue one row per live workspace and prove that the tenant whose
// scan failed was the ONLY row that failed — the per-tenant row was the unit of
// failure. A pass is one row now (ADR-0103 §1), so the property worth pinning is
// the half that never depended on there being several: a sweep that could not
// scan must leave a FAILED row behind, because a pass reporting success is
// indistinguishable from a pass that found nothing due, and what it hides is
// every parked delivery never being re-attempted again.
func TestWebhookRetryRecordsAFailedPassAsAFailedRow(t *testing.T) {
	we := setupWebhooks(t)
	owner := integration.OwnerConn(t)

	rcv := newReceiver(t, http.StatusInternalServerError)
	now := time.Now().UTC()
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())
	parkOneDelivery(t, we, deliverer, rcv)
	// Past the parked delivery's backoff, so the sweep has something due to
	// scan for: a pass with nothing due never reads a row and so never meets
	// the fault.
	now = now.Add(64 * time.Second)
	// Permanent, not transient: the row fires a failure event on every attempt,
	// and a fault that healed would let a later attempt complete and record the
	// pass as green — the exact outcome this test denies.
	failDueScans(t, owner)

	_, completed, failed := jobtest.StartTestJobRunner(t, we.pool, compose.JobRunnerConfig{
		WebhookRetry: compose.WebhookRetryConfig{
			Deliverer: func(db *database.DB) *webhooks.Deliverer {
				return newTestDelivererOn(we, db, &now, rcv.server.Client())
			},
		},
	})
	waitCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	kind := compose.WebhookRetryArgs{}.Kind()
	if outcome := jobtest.AwaitKindOutcome(waitCtx, t, completed, failed, kind); outcome {
		t.Error("the pass whose due scan could not run reported a completed job — the failure the row exists to record was swallowed")
	}

	// What the ROW records is the classified message — River stores the
	// classified fault and keeps the cause in the process log — so the proof
	// that this failure is the injected one lives next door, in the test that
	// calls the sweep directly and can read the raw error.
}

// TestWebhookRetryDispatchRepeatsOnItsConfiguredInterval pins the half of the
// schedule a boot pass hides. RunOnStart fires once whatever the cadence is, so
// a dispatcher wired to a constant instead of the admin's webhook retry
// setting looks identical at boot and then never runs again —
// every parked delivery in the fleet stranded, with every gate green. Two
// dispatches less than jobtest.DispatchGapBound apart can only happen if a
// cadence far shorter than that setting's own default is what River is scheduling
// on.
func TestWebhookRetryDispatchRepeatsOnItsConfiguredInterval(t *testing.T) {
	we := setupWebhooks(t)
	now := time.Now().UTC()
	rcv := newReceiver(t, http.StatusInternalServerError)

	_, completed, _ := jobtest.StartTestJobRunner(t, we.pool, compose.JobRunnerConfig{
		Schedules: compose.SchedulesForTest(map[string]time.Duration{
			identity.WebhookRetryIntervalSeconds.Key(): jobtest.DispatchInterval,
		}),
		WebhookRetry: compose.WebhookRetryConfig{
			Deliverer: func(*database.DB) *webhooks.Deliverer { return newTestDeliverer(we, &now, rcv.server.Client()) },
		},
	})
	// Generous compared with the gap bound: a run this slow is a sick machine,
	// and the assertion that decides the outcome is the gap below, not this.
	waitCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	kind := compose.WebhookRetryArgs{}.Kind()
	first, second := jobtest.AwaitTwoDispatchArrivals(waitCtx, t, completed, kind)
	if gap := second.Sub(first); gap > jobtest.DispatchGapBound {
		t.Fatalf("the two %s dispatches were %s apart, over the %s bound — the schedule is not the configured %s interval but some larger constant, and the setting's own 30s default is the one that would look exactly like this",
			kind, gap, jobtest.DispatchGapBound, jobtest.DispatchInterval)
	}
}
