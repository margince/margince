// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Why a retirement the claim guard turned away was turned away.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// declinedRetire answers a retirement the claim guard turned away.
//
// This worker retiring the row twice asks for a state it already reached, and
// the no-op is what makes the retry safe.
//
// A claim lost to another worker is the one that matters. The verdict this
// call carries cost a model answer, and answering nil throws it away while the
// caller counts the attempt as spent.
func declinedRetire(ctx context.Context, tx pgx.Tx, id, claim ids.UUID) error {
	var status string
	var claimedBy *ids.UUID
	switch err := tx.QueryRow(ctx,
		`SELECT status, claimed_by FROM capture_pending_counterparty WHERE id = $1`,
		id).Scan(&status, &claimedBy); {
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("%w: capture disposition %s", apperrors.ErrNotFound, id)
	case err != nil:
		return fmt.Errorf("capture: reading the disposition that refused the retirement %s: %w", id, err)
	case status == PendingStatusUnsure:
		return nil
	default:
		held := "nobody"
		if claimedBy != nil {
			held = claimedBy.String()
		}
		return fmt.Errorf("%w: capture disposition %s is %s and claimed by %s, not by %s",
			apperrors.ErrConflict, id, status, held, claim)
	}
}
