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

// PolicyActor binds, in the caller's transaction, the agent that applies a
// proposal for its owner, with the owner's grants as they stand in that
// transaction. ok is false when the record changed hands or the owner is no
// longer live. Members and records are not this module's.
type PolicyActor func(ctx context.Context, tx pgx.Tx) (actor context.Context, ok bool, err error)

// ErrPolicyWithdrawn reports an automatic apply that its owner stopped
// allowing after the apply was decided: they left, lost the record or the
// grant, or turned the kind off. Nothing was written.
var ErrPolicyWithdrawn = fmt.Errorf("the owner no longer lets this change apply without asking: %w",
	apperrors.ErrPermissionDenied)

type underPolicyKey struct{}

// underPolicy names the one approval an automatic apply is releasing, so a
// redemption of any other approval on the same context is not read as one.
type underPolicy struct {
	id    ids.ApprovalID
	actAs PolicyActor
}

func withPolicyActor(ctx context.Context, id ids.ApprovalID, actAs PolicyActor) context.Context {
	return context.WithValue(ctx, underPolicyKey{}, underPolicy{id: id, actAs: actAs})
}

func policyActorFor(ctx context.Context, id ids.ApprovalID) (PolicyActor, bool) {
	held, ok := ctx.Value(underPolicyKey{}).(underPolicy)
	if !ok || held.id != id {
		return nil, false
	}
	return held.actAs, true
}

// actUnderPolicyNow binds the owner's agent as of tx and asks again whether
// the owner still has a's kind on automatic. The decision and the effect each
// ask in their own transaction, because each commits separately and a
// suspension, role change or switch can land between them.
func actUnderPolicyNow(ctx context.Context, tx pgx.Tx, a row) (context.Context, principal.Principal, error) {
	actAs, ok := policyActorFor(ctx, a.ID)
	if !ok || actAs == nil {
		return nil, principal.Principal{}, errors.New(
			"crmapprovals: an automatic apply needs its owner's authority read in its transaction")
	}
	actor, held, err := actAs(ctx, tx)
	if err != nil {
		return nil, principal.Principal{}, err
	}
	if !held {
		return nil, principal.Principal{}, ErrPolicyWithdrawn
	}
	p, ok := principal.Actor(actor)
	if !ok || p.UserID.IsZero() {
		return nil, principal.Principal{}, errors.New("crmapprovals: the owner's agent names no owner")
	}
	mode, err := AutoApplyModeTx(actor, tx, p.UserID, a.Kind)
	if err != nil {
		return nil, principal.Principal{}, err
	}
	if mode != ModeAuto {
		return nil, principal.Principal{}, ErrPolicyWithdrawn
	}
	return actor, p, nil
}

// stillUnderPolicy asks again, inside an effect's redemption, whether the
// owner may still decide a and has its kind on automatic, when that
// redemption releases an automatic apply. Any other redemption passes.
func stillUnderPolicy(ctx context.Context, tx pgx.Tx, a row) error {
	if _, ok := policyActorFor(ctx, a.ID); !ok {
		return nil
	}
	actor, p, err := actUnderPolicyNow(ctx, tx, a)
	if err != nil {
		return err
	}
	visible, err := decidable(actor, tx, p, a)
	if err != nil {
		return err
	}
	if !visible {
		return ErrPolicyWithdrawn
	}
	return nil
}
