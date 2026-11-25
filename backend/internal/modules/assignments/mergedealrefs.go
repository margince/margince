// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assignments

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MoveDealRefsTx moves the role assignments of a merged-away deal onto its
// survivor, in the merge's transaction. It audits each row it changes and
// answers the ids it moved.
func MoveDealRefsTx(ctx context.Context, tx pgx.Tx, from, to ids.UUID) ([]recordmerge.Moved, error) {
	var moved []recordmerge.Moved
	archived, err := recordmerge.MovedIDs(ctx, tx, `UPDATE record_assignment a SET archived_at = now()
 WHERE a.deal_id = $1 AND a.archived_at IS NULL AND EXISTS (
   SELECT 1 FROM record_assignment b
    WHERE b.deal_id = $2 AND b.role_id = a.role_id AND b.archived_at IS NULL
      AND ((a.user_id IS NOT NULL AND b.user_id = a.user_id) OR (a.team_id IS NOT NULL AND b.team_id = a.team_id)))
 RETURNING a.id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "archive", "record_assignment", archived,
		nil, nil); err != nil {
		return nil, fmt.Errorf("audit the record_assignment rows of the merge: %w", err)
	}
	movedIDs, err := recordmerge.MovedIDs(ctx, tx, `UPDATE record_assignment SET deal_id = $2 WHERE deal_id = $1 RETURNING id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", "record_assignment", movedIDs,
		map[string]any{"deal_id": from}, map[string]any{"deal_id": to}); err != nil {
		return nil, fmt.Errorf("audit the record_assignment rows of the merge: %w", err)
	}
	moved = append(moved, recordmerge.Moved{Table: "record_assignment", IDs: movedIDs})
	return moved, nil
}
