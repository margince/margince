// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mayManagePassport decides who may end or release a passport. Another user's
// passport reads as absent, not forbidden, matching the row-scope convention.
//
// Only the CROSS-USER branch is gated. Managing your own passport is
// self-authorized: requiring a member-administration grant would take from
// every seat the ability to cut off an agent acting as them.
func mayManagePassport(ctx context.Context, tx pgx.Tx, id Identity, onBehalfOf ids.UserID) error {
	if onBehalfOf == id.UserID {
		return nil
	}
	if err := auth.Require(ctx, objectUserAdmin, principal.ActionUpdate); err != nil {
		return apperrors.ErrNotFound
	}
	// The same ceiling IssuePasswordLink carries: a delegated holder who could
	// cut off every agent acting for an administrator has reach over that
	// administrator's work.
	if err := refuseUnlessCallerOutranksTarget(ctx, tx, id, onBehalfOf, reachDenial); err != nil {
		return apperrors.ErrNotFound
	}
	return nil
}
