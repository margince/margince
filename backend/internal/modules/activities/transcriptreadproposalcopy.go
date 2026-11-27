// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CopyTranscriptReadProposalsTx copies the proposal_ids of up to limit
// readings after the cursor into transcript_read_proposal. It returns the
// readings it inspected and how many gained a row. An id keeps its first place
// and is left out once its approval is gone; a row already there stays, so a
// replay changes nothing. The page is locked so a finishing reading cannot
// write its list mid-copy.
func CopyTranscriptReadProposalsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	var inspected []ids.UUID
	var changed int
	if err := tx.QueryRow(ctx, `
		WITH page AS (
			SELECT id, proposal_ids FROM transcript_read WHERE id > $1 ORDER BY id LIMIT $2 FOR NO KEY UPDATE
		), copied AS (
			INSERT INTO transcript_read_proposal (transcript_read_id, approval_id, position)
			SELECT page.id, staged.approval_id, min(staged.n)
			  FROM page CROSS JOIN LATERAL unnest(page.proposal_ids) WITH ORDINALITY AS staged(approval_id, n)
			 WHERE EXISTS (SELECT 1 FROM approval a WHERE a.id = staged.approval_id)
			 GROUP BY page.id, staged.approval_id
			ON CONFLICT DO NOTHING
			RETURNING transcript_read_id
		)
		SELECT coalesce((SELECT array_agg(id ORDER BY id) FROM page), '{}'),
		       (SELECT count(DISTINCT transcript_read_id) FROM copied)`,
		after, limit).Scan(&inspected, &changed); err != nil {
		return nil, 0, fmt.Errorf("activities: copying transcript read proposals: %w", err)
	}
	return inspected, changed, nil
}

// CountTranscriptReadsToCopyProposals counts the readings
// CopyTranscriptReadProposalsTx inspects, reading at most maximum+1.
func CountTranscriptReadsToCopyProposals(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM transcript_read LIMIT $1) reads`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting transcript reads: %w", err)
	}
	return count, nil
}
