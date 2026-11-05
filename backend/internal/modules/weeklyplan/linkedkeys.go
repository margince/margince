// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

// The backfill for a commitment's per-type link keys. A row written before the
// keys is keyed, or lets go of a record that is already gone.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// keyLinkedRecordsSQL clears the marker on a page of commitments so the trigger
// keys them. A link whose record is gone is cleared first, as the SET NULL key
// would have done; the commitment itself stays. The marker filter makes a
// replay change nothing.
const keyLinkedRecordsSQL = `
	WITH page AS (
	    SELECT c.id, c.linked_record_id IS NOT NULL AND NOT COALESCE(CASE c.linked_record_type
	             WHEN 'deal'    THEN EXISTS (SELECT 1 FROM deal r    WHERE r.id = c.linked_record_id)
	             WHEN 'lead'    THEN EXISTS (SELECT 1 FROM lead r    WHERE r.id = c.linked_record_id)
	             WHEN 'contact' THEN EXISTS (SELECT 1 FROM contact r WHERE r.id = c.linked_record_id)
	             WHEN 'company' THEN EXISTS (SELECT 1 FROM company r WHERE r.id = c.linked_record_id)
	             WHEN 'project' THEN EXISTS (SELECT 1 FROM project r WHERE r.id = c.linked_record_id)
	           END, false) AS gone
	      FROM weekly_plan_commitment c
	     WHERE c.linked_record_unkeyed AND c.id > $1
	     ORDER BY c.id
	     LIMIT $2
	       FOR UPDATE
	), keyed AS (
	    UPDATE weekly_plan_commitment c
	       SET linked_record_unkeyed = false,
	           linked_record_type = CASE WHEN page.gone THEN NULL ELSE c.linked_record_type END,
	           linked_record_id   = CASE WHEN page.gone THEN NULL ELSE c.linked_record_id END
	      FROM page
	     WHERE c.id = page.id
	    RETURNING c.id
	)
	SELECT id FROM keyed ORDER BY id`

// KeyLinkedRecordsTx keys one page of the commitments written before the link
// keys, after the given id. It returns the page's ids, each of which it changed.
func KeyLinkedRecordsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, keyLinkedRecordsSQL, after, limit)
	if err != nil {
		return nil, fmt.Errorf("weeklyplan: keying linked records: %w", err)
	}
	changed, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("weeklyplan: keying linked records: %w", err)
	}
	return changed, nil
}

// EstimateUnkeyedLinks counts, up to limit+1, the commitments
// KeyLinkedRecordsTx still has to inspect.
func EstimateUnkeyedLinks(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM weekly_plan_commitment WHERE linked_record_unkeyed LIMIT $1) unkeyed`, limit+1).Scan(&count)
	return count, err
}
