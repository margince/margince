// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// flipResolver answers EffectiveRBAC with a live grant until it is revoked, when
// it answers ErrNotFound — the deactivated-owner shape canSee reads as "sees
// nothing". SeatType and AdmittedAuthority are never asked on the delivery path.
type flipResolver struct{ revoked atomic.Bool }

func (r *flipResolver) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	if r.revoked.Load() {
		return authz.RBAC{}, apperrors.ErrNotFound
	}
	return authz.RBAC{Permissions: principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}},
		RowScope: principal.RowScopeAll,
	}}, nil
}

func (r *flipResolver) SeatType(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error) {
	var zero principal.SeatType
	return zero, nil
}

func (r *flipResolver) AdmittedAuthority(context.Context, ids.UUID, ids.UUID, ids.UUID) (authz.RBAC, principal.SeatType, error) {
	var zero principal.SeatType
	return authz.RBAC{}, zero, nil
}

// revocationEnv is one seeded contact delivery, its receiver, and the resolver
// whose answer the test flips mid-send.
type revocationEnv struct {
	deliverer *Deliverer
	resolver  *flipResolver
	received  *atomic.Bool
	target    attemptTarget
}

func (e *contractVisEnv) seedContactDelivery(t *testing.T, resolver authz.Resolver) revocationEnv {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	sub, deliveries := e.seedDeliveries(t, 1)
	contact := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'A buyer', 'manual', 'human:test')`,
		contact); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	if _, err := e.owner.Exec(ctx,
		`UPDATE webhook_delivery SET entity_type = 'contact', entity_id = $2 WHERE id = $1`, deliveries[0], contact); err != nil {
		t.Fatalf("naming the delivery's subject: %v", err)
	}
	received := &atomic.Bool{}
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.Close)
	cipher, err := NewCipher(make([]byte, WebhookKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	secret, err := generateSecret()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.seal(secret)
	if err != nil {
		t.Fatal(err)
	}
	d := NewDeliverer(NewStore(e.store.db, cipher), loopbackClient(transportTLS(t, receiver)),
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, resolver, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return revocationEnv{
		deliverer: d, received: received,
		target: attemptTarget{
			deliveryID: deliveries[0], subID: sub, ownerID: e.user, targetURL: receiver.URL, sealedSecret: sealed,
			eventType: "contact.created", entityType: "contact", entityID: contact, eventID: ids.NewV7(), payload: []byte(`{}`),
		},
	}
}

// A revocation that lands while the connection is being built is caught under
// the hold, and no byte of the body is written. The window the pre-send check
// leaves open is the DNS, TCP and TLS round trip between its answer and the
// write; beforeSend stands in for a revocation that commits inside it.
func TestARevocationDuringTheSendWithholdsTheBody(t *testing.T) {
	e := setupContractVis(t)
	resolver := &flipResolver{}
	env := e.seedContactDelivery(t, resolver)
	env.deliverer.beforeSend = func(context.Context) error { resolver.revoked.Store(true); return nil }

	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	got := env.deliverer.attempt(ctx, env.target, env.deliverer.sendGuard(ctx, env.target))
	if !got.withdrawn {
		t.Fatalf("attempt = %+v, want it withdrawn — a revocation during the send let the body out", got)
	}
	if env.received.Load() {
		t.Fatal("the receiver got the request after the owner lost access mid-send")
	}
}

// The owner who keeps access is still delivered to: the under-hold re-check is a
// gate on a changed answer, not a second toll on every send.
func TestAnOwnerWhoKeepsAccessIsDelivered(t *testing.T) {
	e := setupContractVis(t)
	env := e.seedContactDelivery(t, &flipResolver{})

	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	got := env.deliverer.attempt(ctx, env.target, env.deliverer.sendGuard(ctx, env.target))
	if got != (outcome{statusCode: http.StatusOK}) {
		t.Fatalf("attempt = %+v, want the body delivered", got)
	}
	if !env.received.Load() {
		t.Fatal("the receiver never got a request the owner was authorized for")
	}
}
