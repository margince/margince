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
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type bodyReceiver struct{ posts int }

func (r *bodyReceiver) Do(*http.Request) (*http.Response, error) {
	r.posts++
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestErasureAfterLoadingADeliveryPreventsItsPost(t *testing.T) {
	for _, status := range []string{"pending", "retrying"} {
		t.Run(status, func(t *testing.T) {
			e := setupContractVis(t)
			_, deliveries := e.seedDeliveries(t, 1)
			ctx := e.asHolding(map[string]principal.ObjectGrant{rbacObject: {Read: true, Update: true}})
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
			if _, err := e.owner.Exec(ctx, `UPDATE webhook_delivery SET status=@status, entity_type='company', entity_id=@subject WHERE id=@id`, pgx.NamedArgs{"id": deliveries[0], "subject": e.company, "status": status}); err != nil {
				t.Fatal(err)
			}
			target, err := e.store.loadTarget(ctx, deliveries[0])
			if err != nil {
				t.Fatal(err)
			}
			target.sealedSecret = sealed
			receiver := &bodyReceiver{}
			d := NewDeliverer(e.store, receiver, func() time.Time { return time.Unix(1700000000, 0) }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			hookRan := false
			d.beforeSend = func(ctx context.Context) error {
				hookRan = true
				return e.store.db.Tx(ctx, func(tx pgx.Tx) error {
					return ScrubSubjectDeliveriesTx(ctx, tx, "company", []ids.UUID{e.company})
				})
			}
			d.deliverOnce(ctx, target)
			if !hookRan {
				t.Fatal("the attempt never reached its send gate")
			}
			if receiver.posts != 0 {
				t.Fatal("the receiver got the loaded body after its erasure committed")
			}
			var state, body string
			if err := e.owner.QueryRow(ctx, `SELECT status,payload FROM webhook_delivery WHERE id=@id`, pgx.NamedArgs{"id": deliveries[0]}).Scan(&state, &body); err != nil {
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
	_, deliveries := e.seedDeliveries(t, 1)
	ctx := e.asHolding(map[string]principal.ObjectGrant{rbacObject: {Read: true, Update: true}})
	target, err := e.store.loadTarget(ctx, deliveries[0])
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0)
	d := NewDeliverer(e.store, nil, func() time.Time { return now }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.beforeSend = func(context.Context) error { return errors.New("send check unavailable") }
	if d.sendAuthorized(ctx, target) {
		t.Fatal("a failed send check authorized the request")
	}
	var status, reason string
	var attempts int
	var next time.Time
	if err := e.owner.QueryRow(ctx, `SELECT status, attempts, last_error, next_retry_at FROM webhook_delivery WHERE id=@id`, pgx.NamedArgs{"id": target.deliveryID}).Scan(&status, &attempts, &reason, &next); err != nil {
		t.Fatal(err)
	}
	if status != "retrying" || attempts != target.priorAttempts+1 || reason != visibilityUncheckedReason || !next.After(now) {
		t.Fatalf("failed check did not back off: status=%s attempts=%d reason=%q next=%v", status, attempts, reason, next)
	}
}
