// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func ensureDeliveryThreadAvailableTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var available bool
	if err := tx.QueryRow(ctx, `SELECT activity_thread_available(delivery.thread_key) AND activity_thread_available(source.thread_key)
 FROM comms_outbound delivery JOIN activity source ON source.id = delivery.activity_id WHERE delivery.id = @id`, pgx.NamedArgs{"id": id}).Scan(&available); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTerminal
		}
		return err
	}
	if !available {
		return apperrors.ErrConflict
	}
	return nil
}
