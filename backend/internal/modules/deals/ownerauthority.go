// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OwnerAuthorityTx binds the principal a Margince write acts as for a deal's
// owner, with the grants the owner holds in tx. An owner who is no longer a
// live member answers apperrors.ErrNotFound. Members are not this module's.
type OwnerAuthorityTx func(ctx context.Context, tx pgx.Tx) (context.Context, error)

// actAsOwnerNow binds the owner's authority as of tx and asks require of it
// again. The grants bound before the transaction can predate a role change or
// suspension that committed since. An owner who lost the authority answers
// false, and the write does not happen.
func actAsOwnerNow(
	ctx context.Context, tx pgx.Tx, bind OwnerAuthorityTx, require func(context.Context) error,
) (context.Context, bool, error) {
	if bind == nil {
		return nil, false, errors.New("deals: a write for a deal's owner needs the owner's authority read in its transaction")
	}
	actor, err := bind(ctx, tx)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	err = require(actor)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return actor, true, nil
}

func requireDealUpdate(ctx context.Context) error {
	return auth.Require(ctx, "deal", principal.ActionUpdate)
}
