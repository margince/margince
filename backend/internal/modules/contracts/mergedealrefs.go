// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MoveDealRefsTx moves the contracts of a merged-away deal onto its
// survivor, in the merge's transaction. It audits each row it changes and
// answers the ids it moved.
func MoveDealRefsTx(ctx context.Context, tx pgx.Tx, from, to ids.UUID) ([]recordmerge.Moved, error) {
	var moved []recordmerge.Moved
	contracts, err := recordmerge.MovedIDs(ctx, tx, `UPDATE contract SET deal_id = $2 WHERE deal_id = $1 RETURNING id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", contractObject, contracts,
		map[string]any{filterDealID: from}, map[string]any{filterDealID: to}); err != nil {
		return nil, fmt.Errorf("audit the contract rows of the merge: %w", err)
	}
	moved = append(moved, recordmerge.Moved{Table: contractObject, IDs: contracts})
	return moved, nil
}
