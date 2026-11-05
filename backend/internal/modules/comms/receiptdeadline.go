// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OverdueControllerReceipts exposes missed duties without exposing message contents.
func (s *Store) OverdueControllerReceipts(ctx context.Context, now time.Time) (int, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return 0, apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{now}
		return tx.QueryRow(ctx, `SELECT count(*) FROM comms_outbound WHERE sender_kind='controller' AND status <> 'sent' AND delivery_due_at < `+storekit.Placeholders(args), args...).Scan(&count)
	})
	return count, err
}
