// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

// Who a subscription runs under. Every delivery is judged against its owner's
// rights, so an owner who leaves stops the subscription. It can be given a new
// owner by an edit or by handing the member's work over; both write through
// setOwner.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OwnerAdmission admits the caller to make next the owner a subscription runs
// as. A delivery reaches whatever its owner may see, so naming an owner is
// acting for them. It takes the authority over that member a handover takes.
type OwnerAdmission func(ctx context.Context, tx pgx.Tx, next ids.UserID) error

// WithOwnerAdmission wires the member-administration check an owner change
// asks. A store without one refuses every change of owner.
func (s *Store) WithOwnerAdmission(admit OwnerAdmission) *Store {
	clone := *s
	clone.admitOwner = admit
	return &clone
}

func (s *Store) ownerAdmission() OwnerAdmission {
	if s.admitOwner != nil {
		return s.admitOwner
	}
	return func(context.Context, pgx.Tx, ids.UserID) error { return apperrors.ErrPermissionDenied }
}

// setOwner adds a change of owner to an update's patch. Only a CHANGE is an
// assignment, so re-stating the current owner passes even after they left.
// admit is asked about the new owner; nil means the caller was admitted for
// the member already, as a handover is. The signing secret stays.
func setOwner(
	ctx context.Context, tx pgx.Tx, p *storekit.Patch, current Subscription, next *ids.UserID, admit OwnerAdmission,
) error {
	if next == nil || next.UUID == current.OwnerID {
		return nil
	}
	if admit != nil {
		if err := admit(ctx, tx, *next); err != nil {
			return err
		}
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

// TransferSubscriptionOwnershipTx moves every live subscription from `from` to
// `to` and answers how many moved. Each goes through the patch and audit an
// edit of its owner writes. Archived subscriptions stay where they are. The
// caller runs it inside a handover, which already admitted them for both
// members.
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
	moved := 0
	for _, id := range owned {
		ok, err := reassignSubscription(ctx, tx, id, from, to)
		if err != nil {
			return 0, err
		}
		if ok {
			moved++
		}
	}
	return moved, nil
}

// reassignSubscription moves one subscription from from to to, and reports
// whether it did. The owner is judged again under the row lock. A handover
// that got there first has moved it already, and this one leaves it with that
// successor.
func reassignSubscription(ctx context.Context, tx pgx.Tx, id ids.UUID, from, to ids.UserID) (bool, error) {
	lock, err := storekit.LockRow(ctx, tx, "webhook_subscription", id, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	current, err := scanSubscription(tx.QueryRow(ctx,
		"SELECT "+subscriptionColumns+" FROM webhook_subscription WHERE id = $1", id))
	if err != nil || current.OwnerID != from.UUID {
		return false, err
	}
	p := storekit.NewPatch()
	if err := setOwner(ctx, tx, p, current, &to, nil); err != nil || p.Empty() {
		return false, err
	}
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return false, err
	}
	_, err = storekit.Audit(ctx, tx, "update", rbacObject, id, p.Before(), p.After())
	return err == nil, err
}
