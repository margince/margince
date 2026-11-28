// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MoveCompanyRefsTx preserves the survivor's existing room when both accounts
// have one. Retiring the other association stops its queued synchronization.
func MoveCompanyRefsTx(ctx context.Context, tx pgx.Tx, from, to ids.UUID) error {
	if err := moveMessengerRefsTx(ctx, tx, RecordCompany, from, to); err != nil {
		return err
	}
	source, err := heldLiveLinkOf(ctx, tx, RecordCompany, from)
	if err != nil && err != ErrLinkNotFound {
		return err
	}
	if err == nil {
		target, err := liveLinkOf(ctx, tx, RecordCompany, to)
		if err != nil && err != ErrLinkNotFound {
			return err
		}
		if err == nil && target.ID != source.ID {
			if err := retireLink(ctx, tx, source); err != nil {
				return err
			}
		}
	}
	rows, err := tx.Query(ctx, `UPDATE chat_link SET company_id=$2 WHERE company_id=$1 RETURNING id`, from, to)
	if err != nil {
		return err
	}
	moved, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	return storekit.AuditEach(ctx, tx, "update", chatLinkEntity, moved, map[string]any{companyIDKey: from}, map[string]any{"company_id": to})
}
