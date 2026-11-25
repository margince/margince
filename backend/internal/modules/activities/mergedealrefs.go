// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MoveDealRefsTx moves the retention evidence and the document hides of a merged-away deal onto its
// survivor, in the merge's transaction. It audits each row it changes and
// answers the ids it moved.
func MoveDealRefsTx(ctx context.Context, tx pgx.Tx, from, to ids.UUID) ([]recordmerge.Moved, error) {
	var moved []recordmerge.Moved
	droppedEvidence, err := recordmerge.MovedIDs(ctx, tx, `DELETE FROM activity_retention_evidence a WHERE a.deal_id = $1 AND EXISTS (
   SELECT 1 FROM activity_retention_evidence b
    WHERE b.deal_id = $2 AND b.activity_id = a.activity_id AND b.basis = a.basis
      AND b.deal_name IS NOT DISTINCT FROM a.deal_name AND b.project_id IS NOT DISTINCT FROM a.project_id
      AND b.project_name IS NOT DISTINCT FROM a.project_name) RETURNING a.id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "delete", "activity_retention_evidence", droppedEvidence,
		nil, nil); err != nil {
		return nil, fmt.Errorf("audit the activity_retention_evidence rows of the merge: %w", err)
	}
	evidence, err := recordmerge.MovedIDs(ctx, tx, `UPDATE activity_retention_evidence SET deal_id = $2 WHERE deal_id = $1 RETURNING id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", "activity_retention_evidence", evidence,
		map[string]any{"deal_id": from}, map[string]any{"deal_id": to}); err != nil {
		return nil, fmt.Errorf("audit the activity_retention_evidence rows of the merge: %w", err)
	}
	moved = append(moved, recordmerge.Moved{Table: "activity_retention_evidence", IDs: evidence})
	droppedHides, err := recordmerge.MovedIDs(ctx, tx, `DELETE FROM deal_document_hide a WHERE a.deal_id = $1 AND EXISTS (
   SELECT 1 FROM deal_document_hide b WHERE b.deal_id = $2 AND b.attachment_id = a.attachment_id) RETURNING a.attachment_id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "delete", "deal_document_hide", droppedHides,
		nil, nil); err != nil {
		return nil, fmt.Errorf("audit the deal_document_hide rows of the merge: %w", err)
	}
	hides, err := recordmerge.MovedIDs(ctx, tx, `UPDATE deal_document_hide SET deal_id = $2 WHERE deal_id = $1 RETURNING attachment_id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", "deal_document_hide", hides,
		map[string]any{"deal_id": from}, map[string]any{"deal_id": to}); err != nil {
		return nil, fmt.Errorf("audit the deal_document_hide rows of the merge: %w", err)
	}
	moved = append(moved, recordmerge.Moved{Table: "deal_document_hide", IDs: hides})
	return moved, nil
}
