// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MoveDealRefsTx moves the stakeholder edges, the leads qualified into it and the SDR handoffs accepted into it of a merged-away deal onto its
// survivor, in the merge's transaction. It audits each row it changes and
// answers the ids it moved.
func MoveDealRefsTx(ctx context.Context, tx pgx.Tx, from, to ids.UUID) ([]recordmerge.Moved, error) {
	var moved []recordmerge.Moved
	duplicates, err := recordmerge.MovedIDs(ctx, tx, `UPDATE relationship a SET archived_at = now()
 WHERE a.deal_id = $1 AND a.archived_at IS NULL AND EXISTS (
   SELECT 1 FROM relationship b
    WHERE b.deal_id = $2 AND b.kind = a.kind AND b.archived_at IS NULL
      AND b.contact_id IS NOT DISTINCT FROM a.contact_id AND b.role IS NOT DISTINCT FROM a.role)
 RETURNING a.id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "archive", "relationship", duplicates,
		nil, nil); err != nil {
		return nil, fmt.Errorf("audit the relationship rows of the merge: %w", err)
	}
	edges, err := recordmerge.MovedIDs(ctx, tx, `UPDATE relationship SET deal_id = $2 WHERE deal_id = $1 RETURNING id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", "relationship", edges,
		map[string]any{"deal_id": from}, map[string]any{"deal_id": to}); err != nil {
		return nil, fmt.Errorf("audit the relationship rows of the merge: %w", err)
	}
	moved = append(moved, recordmerge.Moved{Table: "relationship", IDs: edges})
	leads, err := recordmerge.MovedIDs(ctx, tx, `UPDATE lead SET qualified_deal_id = $2 WHERE qualified_deal_id = $1 RETURNING id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", "lead", leads,
		map[string]any{"qualified_deal_id": from}, map[string]any{"qualified_deal_id": to}); err != nil {
		return nil, fmt.Errorf("audit the lead rows of the merge: %w", err)
	}
	moved = append(moved, recordmerge.Moved{Table: "lead", IDs: leads})
	handoffs, err := recordmerge.MovedIDs(ctx, tx, `UPDATE sdr_handoff SET deal_id = $2 WHERE deal_id = $1 RETURNING id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", "sdr_handoff", handoffs,
		map[string]any{"deal_id": from}, map[string]any{"deal_id": to}); err != nil {
		return nil, fmt.Errorf("audit the sdr_handoff rows of the merge: %w", err)
	}
	moved = append(moved, recordmerge.Moved{Table: "sdr_handoff", IDs: handoffs})
	return moved, nil
}
