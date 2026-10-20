// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func demotionTargetDecidable(ctx context.Context, tx pgx.Tx, targetType *string, targetID *ids.UUID) (bool, error) {
	if targetType == nil || *targetType != tableLead || targetID == nil {
		return false, nil
	}
	readable, err := probeAnswer(auth.Require(ctx, tableLead, principal.ActionRead))
	if err != nil || !readable {
		return false, err
	}
	visible, err := probeAnswer(auth.EnsureWritable(ctx, tx, tableLead, *targetID))
	if err != nil || !visible {
		return false, err
	}
	// Promotion archives the lead; only that reversible state admits this action.
	var promoted bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM lead WHERE id = @lead AND status = 'promoted'
		AND archived_at IS NOT NULL AND promoted_contact_id IS NOT NULL)`,
		pgx.NamedArgs{"lead": *targetID}).Scan(&promoted)
	return promoted, err
}
