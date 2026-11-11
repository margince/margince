// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAFirstAttemptKeepsItsLeaseWhileWaitingForTheReceiver(t *testing.T) {
	e := setupContractVis(t)
	ctx := e.asHolding(map[string]principal.ObjectGrant{rbacObject: {Read: true, Update: true}})
	written, release := make(chan struct{}), make(chan struct{})
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		close(written)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	seed := sendableTarget(ctx, t, e, "delivered", receiver)
	targets, err := e.store.enqueueForSubscriptions(ctx, []enqueueRow{{subID: seed.subID, ownerID: e.user, payload: []byte("{}")}}, "contact.created", ids.NewV7(), "company", e.company)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].claim == nil {
		t.Fatalf("first attempt has no lease: %+v", targets)
	}
	target := targets[0]
	target.sealedSecret, target.targetURL = seed.sealedSecret, receiver.URL
	now := time.Unix(1700000000, 0)
	if _, err := e.owner.Exec(ctx, `UPDATE webhook_delivery SET created_at=@at WHERE id=@id`, pgx.StrictNamedArgs{"at": now.Add(-time.Hour), "id": target.deliveryID}); err != nil {
		t.Fatal(err)
	}
	d := NewDeliverer(e.store, loopbackClient(transportTLS(t, receiver)), func() time.Time { return now }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	done := make(chan struct{})
	go func() { defer close(done); d.deliverOnce(ctx, target) }()
	defer func() { close(release); <-done }()
	select {
	case <-written:
	case <-done:
		t.Fatal("the first attempt never reached the receiver")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, _, found, err := e.store.claimDueRetry(ctx, now); err != nil || found {
		t.Fatalf("sweep claimed an in-flight first attempt: found=%v err=%v", found, err)
	}
}

func TestAFirstSendCannotOutrunItsRemainingLease(t *testing.T) {
	e := setupContractVis(t)
	ctx := e.asHolding(map[string]principal.ObjectGrant{rbacObject: {Read: true, Update: true}})
	receiver, posts := countingReceiver(t)
	target := sendableTarget(ctx, t, e, "pending", receiver)
	var claim time.Time
	if err := e.owner.QueryRow(ctx, `UPDATE webhook_delivery SET claimed_until=now()+interval '5 seconds'
 WHERE id=@delivery RETURNING claimed_until`, pgx.StrictNamedArgs{"delivery": target.deliveryID}).Scan(&claim); err != nil {
		t.Fatal(err)
	}
	target.claim = &claim
	d := NewDeliverer(e.store, loopbackClient(transportTLS(t, receiver)), func() time.Time { return time.Unix(1700000000, 0) }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.deliverOnce(ctx, target)
	if posts.Load() != 0 {
		t.Fatal("a first send started without enough lease time for its request")
	}
	delivery, err := e.store.getDelivery(ctx, target.deliveryID)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.Status != "pending" {
		t.Fatalf("withdrawn first attempt changed state: %s", delivery.Status)
	}
}
