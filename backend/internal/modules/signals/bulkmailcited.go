// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// citesBulkMail admits a signal aliased s that cites a newsletter
// (relstrength.BulkMailSQL) as its evidence.
var citesBulkMail = `EXISTS (SELECT 1 FROM jsonb_array_elements(s.evidence) cited
	  JOIN activity x ON x.id = CASE WHEN cited->>'source_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
	                                 THEN (cited->>'source_id')::uuid END
	 WHERE cited->>'source_type' = 'activity' AND ` + relstrength.BulkMailSQL("x") + `)`

// CountDerivedSignalsOfKind counts the live derived signals of one kind,
// reading at most maximum+1: the rows ArchiveCitingBulkMailTx inspects.
func CountDerivedSignalsOfKind(ctx context.Context, q datatask.Querier, kind string, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM signal s
		 WHERE s.kind = $1 AND s.source_channel = 'derived' AND s.archived_at IS NULL LIMIT $2) live`,
		kind, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("signals: counting %s signals: %w", kind, err)
	}
	return count, nil
}

// ArchiveCitingBulkMailTx reads the next limit live derived signals of one
// kind after the cursor and archives each that cites a newsletter. Such a
// finding was drawn from mail no human wrote, so it states nothing about the
// relationship. It answers every id it inspected, for the cursor, and how many
// it archived.
func ArchiveCitingBulkMailTx(ctx context.Context, tx pgx.Tx, kind string, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT s.id, `+citesBulkMail+` FROM signal s
		 WHERE s.kind = $1 AND s.source_channel = 'derived' AND s.archived_at IS NULL AND s.id > $2
		 ORDER BY s.id LIMIT $3 FOR UPDATE OF s`, kind, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("signals: reading %s signals: %w", kind, err)
	}
	var inspected, cited []ids.UUID
	for rows.Next() {
		var id ids.UUID
		var bulk bool
		if err := rows.Scan(&id, &bulk); err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("signals: reading %s signals: %w", kind, err)
		}
		inspected = append(inspected, id)
		if bulk {
			cited = append(cited, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("signals: reading %s signals: %w", kind, err)
	}
	for _, id := range cited {
		if err := archiveSignalTx(ctx, tx, id); err != nil {
			return nil, 0, fmt.Errorf("signals: archiving signal %s: %w", id, err)
		}
	}
	return inspected, len(cited), nil
}
