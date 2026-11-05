// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

// The backfill for signal's per-type entity keys. A row written before the keys
// is keyed, or deleted when the record it names is already gone.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// keyEntitiesSQL deletes a page's signals whose record is gone, as the cascade
// would have. It clears the marker on the rest, so the trigger keys them. The
// marker filter makes a replay change nothing.
const keyEntitiesSQL = `
	WITH page AS (
	    SELECT s.id, s.entity_id IS NOT NULL AND NOT COALESCE(CASE s.entity_type
	             WHEN 'deal'    THEN EXISTS (SELECT 1 FROM deal r    WHERE r.id = s.entity_id)
	             WHEN 'company' THEN EXISTS (SELECT 1 FROM company r WHERE r.id = s.entity_id)
	             WHEN 'contact' THEN EXISTS (SELECT 1 FROM contact r WHERE r.id = s.entity_id)
	             WHEN 'project' THEN EXISTS (SELECT 1 FROM project r WHERE r.id = s.entity_id)
	           END, false) AS gone
	      FROM signal s
	     WHERE s.entity_unkeyed AND s.id > $1
	     ORDER BY s.id
	     LIMIT $2
	       FOR UPDATE
	), dropped AS (
	    DELETE FROM signal s USING page WHERE s.id = page.id AND page.gone RETURNING s.id
	), keyed AS (
	    UPDATE signal s SET entity_unkeyed = false
	      FROM page WHERE s.id = page.id AND NOT page.gone
	    RETURNING s.id
	)
	SELECT id FROM dropped UNION ALL SELECT id FROM keyed ORDER BY id`

// KeyEntitiesTx keys one page of the signals written before the entity keys,
// after the given id. It returns the page's ids, each of which it changed.
// The touch trigger skips an update that changes only the keys and the marker,
// so a version a lifecycle approval pinned stays current.
func KeyEntitiesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, keyEntitiesSQL, after, limit)
	if err != nil {
		return nil, fmt.Errorf("signals: keying entities: %w", err)
	}
	changed, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("signals: keying entities: %w", err)
	}
	return changed, nil
}

// EstimateUnkeyedEntities counts, up to limit+1, the signals KeyEntitiesTx
// still has to inspect.
func EstimateUnkeyedEntities(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, "signal", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM signal WHERE entity_unkeyed LIMIT $1) unkeyed`, limit+1).Scan(&count)
	return count, err
}
