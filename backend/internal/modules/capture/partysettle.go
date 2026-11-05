// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *Sink) settlePartyAdmission(ctx context.Context, activity ids.ActivityID, source partySource) error {
	if source.Receipt != nil {
		if s.partyReceiptSettlement == nil {
			return errors.New("capture: sent-receipt settlement is not configured")
		}
		return s.partyReceiptSettlement(ctx, *source.Receipt)
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE capture_party_source SET admission_pending=false
 WHERE activity_id=@activity AND user_id=@user AND provider=@provider AND admission_generation=@generation`,
			pgx.NamedArgs{partyActivityArg: activity, partyUserArg: source.User, partyProviderArg: source.Provider, "generation": source.Generation})
		return err
	})
}
