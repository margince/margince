// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MoveDealRefsTx moves the chat links of a merged-away deal onto its survivor.
// A live link leaves the retired deal's project and takes the survivor's under
// a new link's rule. The project joins when it has no channel of its own and
// the caller may edit it. The old project leaves in the same statement as the
// move. A project both deals share is then not mistaken for one that already
// has a channel.
func MoveDealRefsTx(ctx context.Context, tx pgx.Tx, from, to ids.UUID) ([]recordmerge.Moved, error) {
	movedIDs, err := recordmerge.MovedIDs(ctx, tx,
		`UPDATE chat_link SET deal_id = $2,
		        project_id = CASE WHEN archived_at IS NULL THEN NULL ELSE project_id END
		  WHERE deal_id = $1 RETURNING id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", chatLinkEntity, movedIDs,
		map[string]any{"deal_id": from}, map[string]any{"deal_id": to}); err != nil {
		return nil, fmt.Errorf("audit the chat links moving to the merge survivor: %w", err)
	}
	_, project, err := linkedRecords(ctx, tx, RecordDeal, to)
	if err != nil || project == nil || len(movedIDs) == 0 {
		return []recordmerge.Moved{{Table: "chat_link", IDs: movedIDs}}, err
	}
	joined, err := recordmerge.MovedIDs(ctx, tx,
		`UPDATE chat_link SET project_id = $2
		  WHERE id = ANY($1) AND archived_at IS NULL RETURNING id`, movedIDs, *project)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", chatLinkEntity, joined,
		map[string]any{"project_id": nil}, map[string]any{"project_id": *project}); err != nil {
		return nil, fmt.Errorf("audit the chat links taking the survivor's project: %w", err)
	}
	return []recordmerge.Moved{{Table: "chat_link", IDs: movedIDs}}, nil
}
