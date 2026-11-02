// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ThreadHoldActivityTx locks before the import owner writes its contribution;
// a repair only narrows, preserving audiences and holds already on the row.
func ThreadHoldActivityTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, reason string) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	var audience string
	var storedReason *string
	err := tx.QueryRow(ctx, `SELECT audience,audience_reason FROM activity
 WHERE id=@id AND archived_at IS NULL AND restricted_at IS NULL FOR UPDATE`, pgx.NamedArgs{"id": id}).Scan(&audience, &storedReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if audience != audienceWorkspace {
		return true, nil
	}
	return true, writeDerivedAudienceWithRepairTx(ctx, tx, id, audience, storedReason, audienceParticipants, reason, true)
}
