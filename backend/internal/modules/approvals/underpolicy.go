// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PolicyActor is how an automatic apply acts for a proposal's owner in each
// of its transactions. Members and records are not this module's.
type PolicyActor interface {
	// Bind binds the owner's agent with the grants the owner holds in tx. It
	// is the transaction's first statement. ok is false for an owner who is no
	// longer live.
	Bind(ctx context.Context, tx pgx.Tx) (actor context.Context, ok bool, err error)
	// HoldTarget locks the proposal's record until tx ends and reports whether
	// the bound owner still holds it, so a handover waits for the write.
	HoldTarget(ctx context.Context, tx pgx.Tx) (ok bool, err error)
}

// ErrPolicyWithdrawn reports an automatic apply that its owner stopped
// allowing after the apply was decided. They left, lost the record or the
// grant, or turned the kind off. Nothing was written.
var ErrPolicyWithdrawn = fmt.Errorf("the owner no longer lets this change apply without asking: %w",
	apperrors.ErrPermissionDenied)

type underPolicyKey struct{}

// underPolicy names the one approval an automatic apply is releasing. A
// redemption of any other approval on the same context is not read as one.
// admits is the decider check a redemption asks only for that approval, so
// an ordinary redemption stays bounded by the token alone.
type underPolicy struct {
	id     ids.ApprovalID
	actAs  PolicyActor
	admits func(context.Context, pgx.Tx, principal.Principal, row) (bool, error)
}

func withPolicyActor(ctx context.Context, id ids.ApprovalID, actAs PolicyActor) context.Context {
	return context.WithValue(ctx, underPolicyKey{}, underPolicy{id: id, actAs: actAs, admits: decidable})
}

func policyActorFor(ctx context.Context, id ids.ApprovalID) (underPolicy, bool) {
	held, ok := ctx.Value(underPolicyKey{}).(underPolicy)
	if !ok || held.id != id || held.actAs == nil {
		return underPolicy{}, false
	}
	return held, true
}

// bindUnderPolicy binds the owner's agent as of tx for the automatic apply of
// approval id. It keeps the lock order every authorized write keeps: the
// authorization read, then the owner's policy, then the record. The bind waits
// for authorization changes, and removing a member locks that member's
// records, so nothing here may follow a record lock.
func bindUnderPolicy(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) (context.Context, principal.Principal, error) {
	held, ok := policyActorFor(ctx, id)
	if !ok {
		return nil, principal.Principal{}, errors.New(
			"crmapprovals: an automatic apply needs its owner's authority read in its transaction")
	}
	actor, bound, err := held.actAs.Bind(ctx, tx)
	if err != nil {
		return nil, principal.Principal{}, err
	}
	if !bound {
		return nil, principal.Principal{}, ErrPolicyWithdrawn
	}
	p, ok := principal.Actor(actor)
	if !ok || p.UserID.IsZero() {
		return nil, principal.Principal{}, errors.New("crmapprovals: the owner's agent names no owner")
	}
	// A plain read takes no row lock, and an approval's kind never changes.
	var kind string
	if err := tx.QueryRow(ctx, `SELECT kind FROM approval WHERE id = $1`, id).Scan(&kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, principal.Principal{}, apperrors.ErrNotFound
		}
		return nil, principal.Principal{}, err
	}
	if err := stillOnAutomatic(actor, tx, p, kind); err != nil {
		return nil, principal.Principal{}, err
	}
	holds, err := held.actAs.HoldTarget(actor, tx)
	if err != nil {
		return nil, principal.Principal{}, err
	}
	if !holds {
		return nil, principal.Principal{}, ErrPolicyWithdrawn
	}
	return actor, p, nil
}

// stillOnAutomatic asks again whether the bound owner has kind on automatic.
// The decision and the effect each ask in their own transaction, because each
// commits separately and a switch can be turned off between them.
func stillOnAutomatic(ctx context.Context, tx pgx.Tx, p principal.Principal, kind string) error {
	mode, err := AutoApplyModeTx(ctx, tx, p.UserID, kind)
	if err != nil {
		return err
	}
	if mode != ModeAuto {
		return ErrPolicyWithdrawn
	}
	return nil
}

// bindForRedemption binds the owner's agent first in an effect's redemption
// when that redemption releases an automatic apply. ok is false for any other
// redemption.
func bindForRedemption(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) (context.Context, principal.Principal, bool, error) {
	if _, ok := policyActorFor(ctx, id); !ok {
		return nil, principal.Principal{}, false, nil
	}
	actor, p, err := bindUnderPolicy(ctx, tx, id)
	if err != nil {
		return nil, principal.Principal{}, false, err
	}
	return actor, p, true, nil
}

// stillUnderPolicy asks, in an automatic apply's redemption, whether the
// owner bound there may still decide a.
func stillUnderPolicy(actor context.Context, tx pgx.Tx, p principal.Principal, a row) error {
	held, ok := policyActorFor(actor, a.ID)
	if !ok {
		return errors.New("crmapprovals: an automatic apply's redemption lost its owner")
	}
	visible, err := held.admits(actor, tx, p, a)
	if err != nil {
		return err
	}
	if !visible {
		return ErrPolicyWithdrawn
	}
	return nil
}
