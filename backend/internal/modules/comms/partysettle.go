// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SettlePartyAdmission retires derivation work for one durable send receipt.
func (s *Store) SettlePartyAdmission(ctx context.Context, delivery ids.UUID) error {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var activity, owner ids.UUID
		err := tx.QueryRow(ctx, `SELECT activity_id,user_id FROM comms_outbound WHERE id=@delivery AND sender_kind='user' AND status='sent'`, pgx.NamedArgs{"delivery": delivery}).Scan(&activity, &owner)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		actor, ok := principal.Actor(ctx)
		if !ok {
			return apperrors.ErrPermissionDenied
		}
		if actor.Type != principal.PrincipalSystem {
			if actor.UserID != owner && actor.OnBehalfOf != owner {
				return apperrors.ErrPermissionDenied
			}
		}
		if err := auth.EnsureActivityContentVisibleLive(ctx, tx, activity); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE comms_outbound receipt SET party_admission_pending=false
 WHERE id=@delivery AND status='sent' AND EXISTS(SELECT 1 FROM activity source
 WHERE source.id=receipt.activity_id AND source.restricted_at IS NULL
 AND activity_thread_available(source.thread_key))`, pgx.NamedArgs{"delivery": delivery})
		return err
	})
}
