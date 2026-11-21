// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedVectorSQL is the vector of the embedding row aliased alias, at half
// precision. A row the conversion has not reached yet still holds it at full
// precision, and is read through the cast.
func storedVectorSQL(alias string) string {
	return "coalesce(" + alias + ".embedding_half, " + alias + ".embedding::halfvec)"
}

// embeddingKey is the primary key of one embedding row, and the conversion's
// place in the table.
type embeddingKey struct {
	EntityType string   `json:"entity_type"`
	EntityID   ids.UUID `json:"entity_id"`
	ChunkIx    int      `json:"chunk_ix"`
}

// ConvertToHalfPrecisionTx moves up to limit vectors stored at full precision
// to half precision, in key order after the cursor. Postgres converts the
// stored value; no model is called. It answers the next cursor, how many rows
// it moved, and whether none are left.
func ConvertToHalfPrecisionTx(ctx context.Context, tx pgx.Tx, cursor json.RawMessage, limit int) (json.RawMessage, int, bool, error) {
	var after embeddingKey
	if len(cursor) > 0 {
		if err := json.Unmarshal(cursor, &after); err != nil {
			return nil, 0, false, &storekit.MalformedCursorError{}
		}
	}
	rows, err := tx.Query(ctx, `
		WITH due AS (
			SELECT entity_type, entity_id, chunk_ix FROM embedding
			 WHERE (entity_type, entity_id, chunk_ix) > (@type, @id, @ix) AND embedding_half IS NULL
			 ORDER BY entity_type, entity_id, chunk_ix LIMIT @batch
			 FOR UPDATE)
		UPDATE embedding e SET embedding_half = e.embedding::halfvec, embedding = NULL
		  FROM due
		 WHERE e.entity_type = due.entity_type AND e.entity_id = due.entity_id AND e.chunk_ix = due.chunk_ix
		RETURNING e.entity_type, e.entity_id, e.chunk_ix`,
		pgx.StrictNamedArgs{"type": after.EntityType, "id": after.EntityID, "ix": after.ChunkIx, "batch": limit})
	if err != nil {
		return nil, 0, false, fmt.Errorf("search: moving vectors to half precision: %w", err)
	}
	moved, err := pgx.CollectRows(rows, pgx.RowToStructByPos[embeddingKey])
	if err != nil {
		return nil, 0, false, fmt.Errorf("search: moving vectors to half precision: %w", err)
	}
	if len(moved) == 0 {
		return cursor, 0, true, nil
	}
	last := moved[0]
	for _, key := range moved[1:] {
		if key.after(last) {
			last = key
		}
	}
	next, err := json.Marshal(last)
	return next, len(moved), len(moved) < limit, err
}

// after orders two keys the way the primary key does.
func (k embeddingKey) after(other embeddingKey) bool {
	if k.EntityType != other.EntityType {
		return k.EntityType > other.EntityType
	}
	if k.EntityID != other.EntityID {
		return k.EntityID.String() > other.EntityID.String()
	}
	return k.ChunkIx > other.ChunkIx
}

// CountFullPrecisionVectors counts the vectors still stored at full precision,
// reading at most maximum plus one rows.
func CountFullPrecisionVectors(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var total int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM embedding WHERE embedding_half IS NULL LIMIT $1) due`, maximum+1).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("search: counting vectors at full precision: %w", err)
	}
	return total, nil
}
