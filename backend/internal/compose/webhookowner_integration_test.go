// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// adminWithWebhooks is the harness admin holding the integration grant the
// real admin role carries.
func adminWithWebhooks(e *integration.Env) context.Context {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["webhook_subscription"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	return e.As(e.AdminUser, nil, perms)
}

// A webhook runs under its owner and delivers nothing once they leave. The
// subscription says so, and it can be given to a colleague: the change is
// audited and the signing secret stays.
func TestAWebhookWhoseOwnerLeftSaysSoAndCanBeHandedOn(t *testing.T) {
	e := integration.Setup(t)
	store := webhookStoreOver(e.DB(), nil)
	admin := adminWithWebhooks(e)
	hook := seedWebhook(t, e, e.Rep1, false)

	read := func() webhooks.Subscription {
		t.Helper()
		sub, err := store.GetSubscription(admin, hook)
		if err != nil {
			t.Fatal(err)
		}
		return sub
	}
	if !read().OwnerActive {
		t.Fatal("a subscription whose owner is active reads as stopped")
	}
	deactivate(t, e, e.Rep1)
	if read().OwnerActive {
		t.Fatal("a subscription whose owner was deactivated still reads as delivering")
	}

	gone := ids.From[ids.UserKind](e.Rep1)
	stillGone := webhooks.UpdateSubscriptionInput{OwnerID: &gone}
	if _, err := store.UpdateSubscription(admin, hook, stillGone); err != nil {
		t.Fatalf("re-stating the departed owner is no handover, and was refused: %v", err)
	}
	deactivate(t, e, e.Rep3)
	departed := ids.From[ids.UserKind](e.Rep3)
	_, err := store.UpdateSubscription(admin, hook, webhooks.UpdateSubscriptionInput{OwnerID: &departed})
	if !errors.As(err, new(*auth.AssigneeNotAllowedError)) {
		t.Fatalf("handing the webhook to a deactivated seat answered %v, want it refused", err)
	}

	next := ids.From[ids.UserKind](e.Rep2)
	sub, err := store.UpdateSubscription(admin, hook, webhooks.UpdateSubscriptionInput{OwnerID: &next})
	if err != nil {
		t.Fatalf("handing the webhook to an active colleague: %v", err)
	}
	if sub.OwnerID != e.Rep2 || !sub.OwnerActive {
		t.Fatalf("after the handover the owner is %s (active %v), want the colleague, active", sub.OwnerID, sub.OwnerActive)
	}
	if secret := e.WsScalar(t, `SELECT signing_secret_ref FROM webhook_subscription WHERE id = $1`, hook); secret != "sealed" {
		t.Errorf("the signing secret changed with the owner (%q); the receiver would stop verifying", secret)
	}
	audited := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'webhook_subscription' AND entity_id = $1
		AND before->>'owner_id' = $2 AND after->>'owner_id' = $3`, hook, e.Rep1.String(), e.Rep2.String())
	if audited != 1 {
		t.Errorf("the owner change left %d audit rows naming both owners, want 1", audited)
	}
}

// A delivery reaches whatever its owner may see, so naming an owner is acting
// for that member. A caller who may manage webhooks but not members cannot
// make an administrator the owner to receive what only the administrator
// sees. The owner stays as it was.
func TestAWebhookCannotBeMadeToRunAsSomeoneTheCallerMayNotActFor(t *testing.T) {
	e := integration.Setup(t)
	store := webhookStoreOver(e.DB(), nil)
	hook := seedWebhook(t, e, e.Rep2, false)
	integrator := e.As(e.Rep2, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"webhook_subscription": {Read: true, Create: true, Update: true, Delete: true},
		},
		// Every row, so assigning work to the administrator passes on its own
		// and only the member-administration check stands in the way.
		RowScope: principal.RowScopeAll,
	})

	admin := ids.From[ids.UserKind](e.AdminUser)
	_, err := store.UpdateSubscription(integrator, hook, webhooks.UpdateSubscriptionInput{OwnerID: &admin})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("making an administrator the owner without member administration answered %v, want permission denied", err)
	}
	if got := ownerOf(t, e, "webhook_subscription", hook); got != e.Rep2 {
		t.Fatalf("the refused change moved the owner to %s", got)
	}
}
