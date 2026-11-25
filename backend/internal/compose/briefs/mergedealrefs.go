// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MoveDealRefsTx moves the brief items of a merged-away deal onto its
// survivor, in the merge's transaction. It audits each row it changes and
// answers the ids it moved.
func MoveDealRefsTx(ctx context.Context, tx pgx.Tx, from, to ids.UUID) ([]recordmerge.Moved, error) {
	var moved []recordmerge.Moved
	duplicates, err := recordmerge.MovedIDs(ctx, tx, `DELETE FROM brief_item a WHERE a.deal_id = $1 AND EXISTS (
   SELECT 1 FROM brief_item b WHERE b.deal_id = $2 AND b.brief_run_id = a.brief_run_id) RETURNING a.id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "delete", "brief_item", duplicates,
		nil, nil); err != nil {
		return nil, fmt.Errorf("audit the brief_item rows of the merge: %w", err)
	}
	items, err := recordmerge.MovedIDs(ctx, tx, `UPDATE brief_item SET deal_id = $2 WHERE deal_id = $1 RETURNING id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", "brief_item", items,
		map[string]any{"deal_id": from}, map[string]any{"deal_id": to}); err != nil {
		return nil, fmt.Errorf("audit the brief_item rows of the merge: %w", err)
	}
	moved = append(moved, recordmerge.Moved{Table: "brief_item", IDs: items})
	return moved, nil
}
