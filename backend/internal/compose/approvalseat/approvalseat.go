// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package approvalseat answers what one seat may do with an approval under the
// grants it holds now. The approval notice lane asks it when a notice is
// written, and the reconcile sweep asks it again later.
package approvalseat

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// Context binds one seat's current stored authority. live is false for a seat
// that is gone, which is a race and not a fault.
func Context(ctx context.Context, ident *identity.Service, wsID, seat ids.UUID) (context.Context, bool, error) {
	member, err := authz.MemberPrincipal(ctx, ident, wsID, seat)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("resolving the seat's authority: %w", err)
	}
	return principal.WithActor(ctx, member), true, nil
}

// StillDecidable asks the delivery question again: is this approval still the
// seat's to decide, under the grants it holds now. A reviewer who lost the grant
// since the notice was written no longer keeps a line naming the record.
func StillDecidable(
	ctx context.Context, tx pgx.Tx, ident *identity.Service, svc *approvals.Service, approval ids.ApprovalID, seat ids.UUID,
) (bool, error) {
	ws, err := ident.InstallationWorkspace(ctx)
	if err != nil {
		return false, err
	}
	seatCtx, live, err := Context(ctx, ident, ws.UUID, seat)
	if err != nil || !live {
		return false, err
	}
	decidable, _, _, err := svc.PendingAskOfInTx(seatCtx, tx, approval)
	return decidable, err
}
