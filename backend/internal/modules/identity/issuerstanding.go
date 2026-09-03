// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Whether somebody still holds a permission they held when they issued
// something that outlives the moment.
//
// A share link, once created, keeps serving after its author's seat changes.
// Re-checking the ISSUER at open time is what stops a departed employee's link
// from being a standing grant nobody can see in the role matrix. The check has
// to run against the roles as they stand, which is why it re-evaluates rather
// than reading anything frozen at creation.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// IssuerStillHolds answers whether the named user's roles, as they stand
// today, still allow the object and action.
//
// A seat that is no longer a live member answers false rather than an error: a
// link whose author has left is a link that no longer serves, which is the same
// outcome as a link whose author lost the permission, and the caller has one
// case to handle. Liveness is LiveMemberSQL and both its halves — deactivating
// a seat leaves archived_at NULL, so the archived half alone would keep a
// departed colleague's link serving, which is the exact thing this check
// exists to stop.
//
// The evaluation calls loadGrants, which is what login calls. Written out here
// instead, the merge would be a second reading of the role documents, and the
// two would answer differently the first time a role changed shape.
func IssuerStillHolds(
	ctx context.Context, tx pgx.Tx, userID ids.UserID, object string, action principal.Action,
) (bool, error) {
	var live bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM app_user WHERE id = $1 AND `+LiveMemberSQL("")+`)`,
		userID).Scan(&live); err != nil {
		return false, err
	}
	if !live {
		return false, nil
	}
	_, _, perms, err := loadGrants(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	return perms.Allows(object, action), nil
}
