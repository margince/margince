// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messengeraccounts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// MemberAuthority reloads the sender’s role and seat before using stored credentials.
func MemberAuthority(ctx context.Context, tx pgx.Tx) (context.Context, error) {
	member, ok := principal.Actor(ctx)
	if !ok || member.Type != principal.PrincipalHuman || member.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	rbac, seat, err := identity.HeldAuthorityTx(ctx, tx, member.UserID)
	if err != nil {
		return nil, err
	}
	return principal.WithActor(ctx, authz.WithAuthority(member, rbac, seat)), nil
}
