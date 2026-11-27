// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// olderActivitySource selects the activities whose source predates the fixed
// list. A restricted row refuses every update, so it is moved when it is lifted.
var olderActivitySource = `NOT (` + provenance.ListedRecordSourceSQL("source") + `) AND restricted_at IS NULL`

// CorrectActivitySourcesTx moves one page of older activities onto a channel.
// The writer an older word named moves into source_system where that column is
// free (activity_source_system). A row on the list is left out, so a replay
// changes nothing. Audit-only: activity.updated names no source.
func CorrectActivitySourcesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, error) {
	corrected, err := storekit.CorrectRecordSources(ctx, tx, linkEntityActivity,
		`SELECT id, source, activity_source_channel(source, captured_by) FROM activity
		  WHERE id > $1 AND `+olderActivitySource+` ORDER BY id LIMIT $2 FOR UPDATE`,
		`UPDATE activity r SET source = moved.source,
		        source_system = activity_source_system(r.source, r.source_system, r.source_id, r.kind)
		   FROM unnest($1::uuid[], $2::text[]) AS moved(id, source) WHERE r.id = moved.id`,
		after, limit, runID)
	if err != nil {
		return nil, err
	}
	moved := make([]ids.UUID, len(corrected))
	for i, row := range corrected {
		moved[i] = row.ID
	}
	return moved, nil
}

// CountUnlistedActivitySources counts the activities CorrectActivitySourcesTx
// would still move, reading at most maximum+1.
func CountUnlistedActivitySources(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM activity WHERE `+olderActivitySource+`
		 LIMIT $1) unlisted`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting activities with an unlisted source: %w", err)
	}
	return count, nil
}
