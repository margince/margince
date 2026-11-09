// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (*Service) RequirePrivacyReviewTx(ctx context.Context, tx pgx.Tx, object string) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return apperrors.ErrPermissionDenied
	}
	if err := lockAuthorizationShared(ctx, tx); err != nil {
		return err
	}
	// The same stored policy selects notice recipients and admits the reviewer.
	reviewers, err := PrivacyReviewOperatorsTx(principal.SystemActing(ctx, "system:privacy_review"), tx, object)
	if err != nil {
		return err
	}
	if !slices.Contains(reviewers, ids.From[ids.UserKind](actor.UserID)) {
		return apperrors.ErrPermissionDenied
	}
	return nil
}
