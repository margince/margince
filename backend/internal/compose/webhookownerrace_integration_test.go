// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"log/slog"
	"maps"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// afterFirstAuthorization runs once, right after the fan-out has authorized
// the owner and before anything is enqueued or sent. That is the window in
// which an owner can be deactivated or replaced.
type afterFirstAuthorization struct {
	authz.Resolver
	once sync.Once
	then func()
}

func (r *afterFirstAuthorization) EffectiveRBAC(ctx context.Context, ws, user ids.UUID) (authz.RBAC, error) {
	rbac, err := r.Resolver.EffectiveRBAC(ctx, ws, user)
	r.once.Do(r.then)
	return rbac, err
}

// dealWebhook stores a subscription to deal.updated owned by owner, and
// answers a deliverer whose receiver counts what it was sent.
func dealWebhook(t *testing.T, e *integration.Env, owner ids.UUID, between func()) (*webhooks.Deliverer, agendaWebhookReceiver) {
	t.Helper()
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	store := webhooks.NewStore(e.DB(), cipher)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["webhook_subscription"] = principal.ObjectGrant{Create: true, Read: true}
	if _, _, err := store.CreateSubscription(e.As(owner, nil, perms), webhooks.CreateSubscriptionInput{
		TargetURL: "https://hooks.example/deals", EventTypes: []string{"deal.updated"},
	}); err != nil {
		t.Fatal(err)
	}
	receiver := agendaWebhookReceiver{}
	resolver := &afterFirstAuthorization{Resolver: identity.NewService(e.Pool), then: between}
	return webhooks.NewDeliverer(store, receiver, nil, resolver, slog.Default()), receiver
}

func dealUpdated(deal ids.UUID) kevents.Envelope {
	return kevents.Envelope{
		EventID: ids.NewV7(), Type: "deal.updated", Version: 1, OccurredAt: homeReadTime,
		Actor: kevents.Actor{Type: "system", ID: "system"}, Entity: kevents.EntityRef{Type: "deal", ID: deal},
		Payload: []byte(`{}`),
	}
}

// The first attempt is checked again right before its POST. An owner
// deactivated, or replaced, after the fan-out authorized them gets nothing
// sent under their name; with nothing in between, the event goes out.
func TestAFirstWebhookAttemptIsCheckedAgainBeforeItsPost(t *testing.T) {
	for name, tc := range map[string]struct {
		between func(e *integration.Env)
		posts   int
	}{
		"nothing changes": {between: func(*integration.Env) {}, posts: 1},
		"the owner is deactivated": {between: func(e *integration.Env) {
			e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1)
		}},
		"the owner is replaced": {between: func(e *integration.Env) {
			e.WsExec(t, `UPDATE webhook_subscription SET owner_id = $1`, e.Rep3)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			// The resolver reads the owner's grants from their roles, so the
			// owner holds the seeded rep role, as a real member does.
			e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = 'rep'`, e.Rep1)
			pipeline, open, _ := integration.DealFixture(t, e)
			deal := e.SeedDeal(t, "BAER renewal", pipeline, open, &e.Rep1)
			deliverer, receiver := dealWebhook(t, e, e.Rep1, func() { tc.between(e) })
			if err := deliverer.HandleEvent(context.Background(), dealUpdated(deal)); err != nil {
				t.Fatal(err)
			}
			if got := receiver["/deals"]; got != tc.posts {
				t.Fatalf("the receiver was sent %d delivery(ies), want %d (%s)", got, tc.posts,
					e.WsScalar(t, `SELECT coalesce(string_agg(status || ':' || coalesce(last_error, ''), ';'), 'no row') FROM webhook_delivery`))
			}
		})
	}
}

// Two handovers of one member to different colleagues race. The one that
// commits first keeps the webhook; the other finds it moved and leaves it.
func TestTwoRacingHandoversLeaveAWebhookWithTheFirstSuccessor(t *testing.T) {
	e := integration.Setup(t)
	hook := seedWebhook(t, e, e.Rep1, false)
	admin := adminWithWebhooks(e)
	from := ids.From[ids.UserKind](e.Rep1)

	firstMoved, release := make(chan struct{}), make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- e.DB().Tx(admin, func(tx pgx.Tx) error {
			if _, err := webhooks.TransferSubscriptionOwnershipTx(admin, tx, from, ids.From[ids.UserKind](e.Rep2)); err != nil {
				return err
			}
			close(firstMoved)
			<-release
			return nil
		})
	}()
	<-firstMoved
	secondDone := make(chan error, 1)
	var secondMoved int
	go func() {
		secondDone <- e.DB().Tx(admin, func(tx pgx.Tx) error {
			var err error
			secondMoved, err = webhooks.TransferSubscriptionOwnershipTx(admin, tx, from, ids.From[ids.UserKind](e.Rep3))
			return err
		})
	}()
	waitForLockWaiter(t, nil, `'transactionid', 'tuple'`, "the second handover finished without waiting on the first")
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if got := ownerOf(t, e, "webhook_subscription", hook); got != e.Rep2 {
		t.Fatalf("the webhook ended with %s, want the first successor", got)
	}
	if secondMoved != 0 {
		t.Errorf("the second handover reports %d webhook(s) moved, want none", secondMoved)
	}
}
