// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

// A parked delivery re-asks the visibility question before it ships.
//
// The enqueue gate was always right: ownerCanSee resolves the owner's live RBAC
// and tests the subject against it. What it could not do is stay right. A
// delivery parked on a failing endpoint comes due minutes or hours later, and
// the payload is frozen while the answer to "may this owner read this record"
// is not — so an activity narrowed after enqueue still shipped, on the retry
// sweep and again on an operator's replay.
//
// The subject is an ACTIVITY throughout, because activity is the entity type
// whose visibility can change without anybody's grants changing:
// EnsureActivityContentVisible reads the row's own audience.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestARetryStopsWhenTheSubjectLeavesTheOwnersSight(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusInternalServerError) // endpoint is down
	now := time.Now().UTC()
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())

	subID, _ := we.createSubscription(t, rcv.server.URL+"/hook", []string{"activity.captured"})
	activity := we.seedOpenActivity(t)

	// Enqueued while the activity is workspace-visible, so the enqueue gate
	// admits it and the first attempt goes out and fails.
	if err := deliverer.HandleEvent(context.Background(),
		activityEnvelope(we.wsID, activity)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	assertDeliveryStatus(t, we, subID, "retrying", 1)
	if got := rcv.count.Load(); got != 1 {
		t.Fatalf("endpoint saw %d attempts before narrowing, want 1", got)
	}

	we.narrowActivity(t, activity)

	now = now.Add(64 * time.Second)
	if err := deliverer.SweepOnce(webhookSweepCtx(we.wsID)); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	// The endpoint must not have seen a second attempt, and the row must say
	// why. Asserting the count alone would pass on a sweep that never claimed
	// the delivery at all — a broken due scan and a working gate look identical
	// from the receiver's side.
	if got := rcv.count.Load(); got != 1 {
		t.Errorf("endpoint saw %d attempts after the activity was narrowed; the retry shipped "+
			"a record the subscription owner may no longer read", got)
	}
	assertDeliveryStatus(t, we, subID, "visibility_revoked", 1)
	if reason := we.deliveryError(t, subID); reason == "" {
		t.Error("the delivery was parked with no reason, so an operator reading the row " +
			"cannot tell a revocation from a spent budget")
	}
}

func TestAReplayStopsWhenTheSubjectLeavesTheOwnersSight(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusInternalServerError)
	now := time.Now().UTC()
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())

	subID, _ := we.createSubscription(t, rcv.server.URL+"/hook", []string{"activity.captured"})
	activity := we.seedOpenActivity(t)

	if err := deliverer.HandleEvent(context.Background(),
		activityEnvelope(we.wsID, activity)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	assertDeliveryStatus(t, we, subID, "retrying", 1)

	we.narrowActivity(t, activity)

	// The operator asserts the endpoint is fixed — and it is: the receiver now
	// answers 200. Nothing about the ENDPOINT is wrong any more, which is
	// exactly why replay is the more dangerous of the two paths.
	rcv.setStatus(http.StatusOK)
	// A SYSTEM principal, the same shape the replay suite next door uses, and
	// the sharper case: PrincipalSystem bypasses row scope entirely
	// (ActivityAudienceArm returns TRUE for it). The re-check must therefore
	// build its own context from the subscription's owner rather than inheriting
	// the caller's, or an operator-triggered replay would be the one path that
	// sees everything.
	sysCtx := principal.WithActor(
		principal.WithWorkspaceID(context.Background(), we.wsID),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system"},
	)
	deliveryID := we.deliveryID(t, subID)
	if _, err := deliverer.Replay(sysCtx, we.subUUID(t, subID), deliveryID); err != nil {
		t.Fatalf("replay: %v", err)
	}

	if got := rcv.count.Load(); got != 1 {
		t.Errorf("endpoint saw %d attempts; the replay shipped a record the subscription "+
			"owner may no longer read", got)
	}
	assertDeliveryStatus(t, we, subID, "visibility_revoked", 1)
	// resetForReplay clears last_error and hands back a fresh attempt budget, so
	// a check placed after it would park the row with its reason destroyed.
	if reason := we.deliveryError(t, subID); reason == "" {
		t.Error("the replay parked the delivery with no reason; the check ran after the " +
			"replay reset, which clears last_error")
	}
}

func TestADeliveryWithNoRecordedSubjectIsRefused(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusInternalServerError)
	now := time.Now().UTC()
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())

	subID, _ := we.createSubscription(t, rcv.server.URL+"/hook", []string{"activity.captured"})
	activity := we.seedOpenActivity(t)
	if err := deliverer.HandleEvent(context.Background(),
		activityEnvelope(we.wsID, activity)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	// A row written before the subject columns existed. Nothing narrows here and
	// the activity stays workspace-visible: the refusal is about the row being
	// unidentifiable, not about the record being hidden.
	we.execInWorkspace(t,
		`UPDATE webhook_delivery SET entity_type = NULL, entity_id = NULL
		  WHERE subscription_id = $1`, we.subUUID(t, subID))

	now = now.Add(64 * time.Second)
	if err := deliverer.SweepOnce(webhookSweepCtx(we.wsID)); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := rcv.count.Load(); got != 1 {
		t.Errorf("endpoint saw %d attempts; a delivery whose subject cannot be identified "+
			"cannot be re-checked, so it must not ship", got)
	}
	assertDeliveryStatus(t, we, subID, "visibility_revoked", 1)
}

func TestARetryStillShipsWhenTheSubjectIsStillVisible(t *testing.T) {
	// The direction that catches a gate refusing everything. Three tests above
	// prove deliveries stop; this one proves the ones that should not stop do
	// not — a re-check wired to return false always would pass all three.
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusInternalServerError)
	now := time.Now().UTC()
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())

	subID, _ := we.createSubscription(t, rcv.server.URL+"/hook", []string{"activity.captured"})
	activity := we.seedOpenActivity(t)
	if err := deliverer.HandleEvent(context.Background(),
		activityEnvelope(we.wsID, activity)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	assertDeliveryStatus(t, we, subID, "retrying", 1)

	rcv.setStatus(http.StatusOK)
	now = now.Add(64 * time.Second)
	if err := deliverer.SweepOnce(webhookSweepCtx(we.wsID)); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := rcv.count.Load(); got != 2 {
		t.Errorf("endpoint saw %d attempts, want 2: the activity is still workspace-visible, "+
			"so the retry must go out", got)
	}
	assertDeliveryStatus(t, we, subID, "delivered", 2)
}

// TestARevocationHoldsAgainstALaterSweepAndReplay is the durability half: a
// delivery refused once stays refused, however many times something asks again.
//
// Three separate things would each have to fail for it not to. The check itself
// is the load-bearing one — the activity is still narrowed, so a re-claimed
// delivery is simply re-refused — and behind it sit two structural properties
// nothing can currently drive past: markVisibilityRevoked clears next_retry_at,
// and dueRetries claims only 'retrying'. Mutating either of those alone, or both
// together, leaves this test green, which is exactly the point: the guarantee
// does not rest on any one of them.
//
// What it does prove is the operator-visible outcome. The endpoint is healthy
// for the second half, so any path that let the delivery through would deliver
// it, and the count would move.
func TestARevocationHoldsAgainstALaterSweepAndReplay(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusInternalServerError)
	now := time.Now().UTC()
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())

	subID, _ := we.createSubscription(t, rcv.server.URL+"/hook", []string{"activity.captured"})
	activity := we.seedOpenActivity(t)
	if err := deliverer.HandleEvent(context.Background(),
		activityEnvelope(we.wsID, activity)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	we.narrowActivity(t, activity)
	now = now.Add(64 * time.Second)
	if err := deliverer.SweepOnce(webhookSweepCtx(we.wsID)); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	assertDeliveryStatus(t, we, subID, "visibility_revoked", 1)
	attemptsAtRevocation := rcv.count.Load()

	// The endpoint is healthy now, so a sweep that re-claimed the row would
	// deliver it — the revocation undone by a recovery nobody asked about.
	rcv.setStatus(http.StatusOK)
	now = now.Add(64 * time.Second)
	if err := deliverer.SweepOnce(webhookSweepCtx(we.wsID)); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	assertDeliveryStatus(t, we, subID, "visibility_revoked", 1)

	sysCtx := principal.WithActor(
		principal.WithWorkspaceID(context.Background(), we.wsID),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system"},
	)
	if _, err := deliverer.Replay(sysCtx, we.subUUID(t, subID), we.deliveryID(t, subID)); err != nil {
		t.Fatalf("replay: %v", err)
	}
	assertDeliveryStatus(t, we, subID, "visibility_revoked", 1)
	if got := rcv.count.Load(); got != attemptsAtRevocation {
		t.Errorf("the endpoint saw %d attempts after the revocation, want %d: a delivery "+
			"already refused was let through by a later sweep or replay", got, attemptsAtRevocation)
	}
	if reason := we.deliveryError(t, subID); reason == "" {
		t.Error("a later write cleared the revocation reason, so an operator reading the row " +
			"can no longer tell why it stopped")
	}
}

// activityEnvelope names an activity subject, whose visibility is the row's own
// audience rather than anybody's grants.
func activityEnvelope(wsID ids.UUID, activity ids.UUID) kevents.Envelope {
	env := makeEnvelope(wsID, "activity.captured")
	env.Entity = kevents.EntityRef{Type: "activity", ID: activity}
	return env
}

// seedOpenActivity writes a workspace-visible activity: the enqueue gate admits
// it, so every test here starts from a delivery that was correctly authorized.
func (we *webhookEnv) seedOpenActivity(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	we.execInWorkspace(t, `
		INSERT INTO activity (id, kind, audience, source, captured_by, occurred_at)
		VALUES ($1, 'email', 'workspace', 'gmail', 'connector:gmail:seed', now())`, id)
	return id
}

// narrowActivity limits the activity to its participants, of which the
// subscription owner is not one. This is the change the enqueue gate cannot
// see: no grant moved, no subscription changed, and the owner's RBAC is
// identical — only the row's own audience is different.
func (we *webhookEnv) narrowActivity(t *testing.T, activity ids.UUID) {
	t.Helper()
	if rows := we.execInWorkspace(t,
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, activity); rows != 1 {
		t.Fatalf("narrowing the activity touched %d rows, want 1", rows)
	}
}

func (we *webhookEnv) deliveryID(t *testing.T, subID string) ids.UUID {
	t.Helper()
	var id ids.UUID
	we.inWorkspaceTx(t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM webhook_delivery WHERE subscription_id = $1`,
			we.subUUID(t, subID)).Scan(&id)
	})
	return id
}

// deliveryError reads the reason a delivery was parked with. Empty means the
// row says nothing, which for a revoked delivery is the failure: an operator
// reading the store cannot tell a revocation from a spent budget.
func (we *webhookEnv) deliveryError(t *testing.T, subID string) string {
	t.Helper()
	var reason *string
	we.inWorkspaceTx(t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT last_error FROM webhook_delivery WHERE subscription_id = $1`,
			we.subUUID(t, subID)).Scan(&reason)
	})
	if reason == nil {
		return ""
	}
	return *reason
}

func (we *webhookEnv) subUUID(t *testing.T, subID string) ids.UUID {
	t.Helper()
	return mustParseUUID(t, subID)
}
