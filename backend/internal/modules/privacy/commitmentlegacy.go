// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// Older tasks predate the admission ledger but still quote their source.
func legacyCommitmentDerivatives(ctx context.Context, tx pgx.Tx, sources []ids.UUID) ([]commitmentErasure, error) {
	rows, err := tx.Query(ctx, `SELECT a.source_activity_id, a.source_id, a.id FROM activity a
 WHERE a.source_system = @system AND a.source_activity_id = ANY(@sources::uuid[]) AND a.source_id IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM activity_commitment_admission d
 WHERE d.source_activity_id = a.source_activity_id AND d.locator = a.source_id)
 ORDER BY a.id`, pgx.StrictNamedArgs{"system": provenance.CommitmentTaskSource, "sources": sources})
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (commitmentErasure, error) {
		var item commitmentErasure
		err := row.Scan(&item.source, &item.locator, &item.task)
		return item, err
	})
}
