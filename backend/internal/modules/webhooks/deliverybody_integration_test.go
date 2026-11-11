// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// countingReceiver counts the requests that reach it, behind the real
// delivery client.
func countingReceiver(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var posts atomic.Int64
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		posts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.Close)
	return receiver, &posts
}

// sendableTarget loads one seeded delivery about a company, pointed at the
// receiver and signed with a fresh secret.
func sendableTarget(ctx context.Context, t *testing.T, e *contractVisEnv, status string, receiver *httptest.Server) attemptTarget {
	t.Helper()
	_, deliveries := e.seedDeliveries(t, 1)
	cipher, err := NewCipher(make([]byte, WebhookKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	e.store.cipher = cipher
	secret, err := generateSecret()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.seal(secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `UPDATE webhook_delivery SET status=@status, entity_type='company', entity_id=@subject WHERE id=@id`,
		pgx.NamedArgs{"id": deliveries[0], "subject": e.company, "status": status}); err != nil {
		t.Fatal(err)
	}
	target, err := e.store.loadTarget(ctx, deliveries[0])
	if err != nil {
		t.Fatal(err)
	}
	target.sealedSecret, target.targetURL = sealed, receiver.URL
	return target
}

func TestErasureAfterLoadingADeliveryPreventsItsPost(t *testing.T) {
	for _, status := range []string{"pending", "retrying"} {
		t.Run(status, func(t *testing.T) {
			e := setupContractVis(t)
			ctx := e.asHolding(map[string]principal.ObjectGrant{rbacObject: {Read: true, Update: true}})
			receiver, posts := countingReceiver(t)
			target := sendableTarget(ctx, t, e, status, receiver)
			d := NewDeliverer(e.store, loopbackClient(transportTLS(t, receiver)),
				func() time.Time { return time.Unix(1700000000, 0) }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			hookRan := false
			d.beforeSend = func(ctx context.Context) error {
				hookRan = true
				return e.store.db.Tx(ctx, func(tx pgx.Tx) error {
					return ScrubSubjectDeliveriesTx(ctx, tx, "company", []ids.UUID{e.company})
				})
			}
			d.deliverOnce(ctx, target)
			if !hookRan {
				t.Fatal("the attempt never reached its send check")
			}
			if posts.Load() != 0 {
				t.Fatal("the receiver got the loaded body after its erasure committed")
			}
			var state, body string
			if err := e.owner.QueryRow(ctx, `SELECT status,payload FROM webhook_delivery WHERE id=@id`, pgx.NamedArgs{"id": target.deliveryID}).Scan(&state, &body); err != nil {
				t.Fatal(err)
			}
			if state != "visibility_revoked" || body != "" {
				t.Fatalf("erasure lost: %s / %q", state, body)
			}
		})
	}
}

func TestAFailedSendCheckBacksOffWithoutPosting(t *testing.T) {
	e := setupContractVis(t)
	ctx := e.asHolding(map[string]principal.ObjectGrant{rbacObject: {Read: true, Update: true}})
	receiver, posts := countingReceiver(t)
	target := sendableTarget(ctx, t, e, "pending", receiver)
	now := time.Unix(1700000000, 0)
	d := NewDeliverer(e.store, loopbackClient(transportTLS(t, receiver)), func() time.Time { return now }, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.beforeSend = func(context.Context) error { return errors.New("send check unavailable") }
	d.deliverOnce(ctx, target)
	if posts.Load() != 0 {
		t.Fatal("a failed send check let the request through")
	}
	var status, reason string
	var attempts int
	var next time.Time
	if err := e.owner.QueryRow(ctx, `SELECT status, attempts, last_error, next_retry_at FROM webhook_delivery WHERE id=@id`, pgx.NamedArgs{"id": target.deliveryID}).Scan(&status, &attempts, &reason, &next); err != nil {
		t.Fatal(err)
	}
	if status != "retrying" || attempts != target.priorAttempts+1 || reason != sendUncheckedReason || !next.After(now) {
		t.Fatalf("failed check did not back off: status=%s attempts=%d reason=%q next=%v", status, attempts, reason, next)
	}
}

func TestAnAbandonedPendingDeliveryCanBeClaimedOnlyAfterItsLeaseWindow(t *testing.T) {
	e := setupContractVis(t)
	ctx := e.asHolding(map[string]principal.ObjectGrant{rbacObject: {Read: true, Update: true}})
	receiver, posts := countingReceiver(t)
	target := sendableTarget(ctx, t, e, "pending", receiver)
	now := time.Unix(1700000000, 0).UTC()
	if _, err := e.owner.Exec(ctx, `UPDATE webhook_delivery SET created_at=@now, attempts=0 WHERE id=@id`,
		pgx.StrictNamedArgs{"now": now, "id": target.deliveryID}); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := e.store.claimDueRetry(ctx, now); err != nil || found {
		t.Fatalf("fresh pending claimed: %v %v", found, err)
	}
	id, claim, found, err := e.store.claimDueRetry(ctx, now.Add(claimLease+time.Second))
	if err != nil || !found || id != target.deliveryID {
		t.Fatalf("abandoned pending not recovered: %v %v %v", id, found, err)
	}
	if _, _, found, err := e.store.claimDueRetry(ctx, now.Add(claimLease+time.Second)); err != nil || found {
		t.Fatalf("leased pending claimed twice: %v %v", found, err)
	}
	target.claim = &claim
	target.priorAttempts = 0
	d := NewDeliverer(e.store, loopbackClient(transportTLS(t, receiver)), func() time.Time { return now }, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.deliverOnce(ctx, target)
	delivery, err := e.store.getDelivery(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 || delivery.Status != "delivered" || delivery.Attempts != 1 {
		t.Fatalf("recovery failed: posts=%d delivery=%+v", posts.Load(), delivery)
	}
}
