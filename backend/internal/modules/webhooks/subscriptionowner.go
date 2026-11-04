// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

// Who a subscription runs under. Every delivery is judged against its owner's
// rights, so an owner who leaves stops the subscription. It can be given a new
// owner by an edit or by handing the member's work over; both write through
// setOwner.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// setOwner adds a change of owner to an update's patch. Only a CHANGE is an
// assignment, so re-stating the current owner passes even after they left.
// The signing secret stays: the receiver keeps verifying.
func setOwner(ctx context.Context, tx pgx.Tx, p *storekit.Patch, current Subscription, next *ids.UserID) error {
	if next == nil {
		return nil
	}
	if err := auth.EnsureOwnerHandOn(ctx, tx, &current.OwnerID, next); err != nil {
		return err
	}
	p.Set("owner_id", current.OwnerID, next.UUID)
	return nil
}

// OwnedLiveSubscriptionsTx counts the live subscriptions owner holds, for a
// handover's preview. A caller who may not read subscriptions counts none.
func OwnedLiveSubscriptionsTx(ctx context.Context, tx pgx.Tx, owner ids.UserID) (int, error) {
	if !auth.Allows(ctx, rbacObject, principal.ActionRead) {
		return 0, nil
	}
	var n int
	err := tx.QueryRow(ctx,
		`SELECT count(*) FROM webhook_subscription WHERE owner_id = $1 AND archived_at IS NULL`, owner).Scan(&n)
	return n, err
}

// TransferSubscriptionOwnershipTx moves every live subscription from owns to
// to, each through the same patch and audit an edit of its owner writes, and
// answers how many moved. Archived subscriptions stay where they are.
func TransferSubscriptionOwnershipTx(ctx context.Context, tx pgx.Tx, from, to ids.UserID) (int, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx,
		`SELECT id FROM webhook_subscription WHERE owner_id = $1 AND archived_at IS NULL ORDER BY id`, from)
	if err != nil {
		return 0, err
	}
	owned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, err
	}
	for _, id := range owned {
		if err := reassignSubscription(ctx, tx, id, to); err != nil {
			return 0, err
		}
	}
	return len(owned), nil
}

func reassignSubscription(ctx context.Context, tx pgx.Tx, id ids.UUID, to ids.UserID) error {
	lock, err := storekit.LockRow(ctx, tx, "webhook_subscription", id, storekit.LiveOnly)
	if err != nil {
		return err
	}
	current, err := scanSubscription(tx.QueryRow(ctx,
		"SELECT "+subscriptionColumns+" FROM webhook_subscription WHERE id = $1", id))
	if err != nil {
		return err
	}
	p := storekit.NewPatch()
	if err := setOwner(ctx, tx, p, current, &to); err != nil || p.Empty() {
		return err
	}
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return err
	}
	_, err = storekit.Audit(ctx, tx, "update", rbacObject, id, p.Before(), p.After())
	return err
}
