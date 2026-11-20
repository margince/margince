// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// connectionEntitySQL is the sending company of the mailbox an import came
// from: its own connection for a shared mailbox. For a seat, the live
// connections whose mailbox is named in delivered_by (the mark recordImportTx
// wrote), counted only when they agree on one company. Anything unclear, or an
// owner-less connection, leaves the import unscoped.
var connectionEntitySQL = `coalesce(
	(SELECT c.legal_entity_id FROM capture_connection c WHERE c.id = i.connection_id),
	(SELECT CASE WHEN count(DISTINCT c.legal_entity_id) = 1 AND count(*) = count(c.legal_entity_id)
	              THEN (array_agg(c.legal_entity_id))[1] END
	   FROM capture_connection c
	  WHERE c.user_id = i.user_id AND c.archived_at IS NULL
	    AND ` + storekit.MailboxSQL("c") + ` IN (SELECT split_part(mark, ':', 1) FROM unnest(i.delivered_by) mark)))`

// unscopedImportsSQL selects imports with no sending company whose mailbox has one.
var unscopedImportsSQL = `FROM capture_import i WHERE i.legal_entity_id IS NULL AND ` + connectionEntitySQL + ` IS NOT NULL`

// ScopeImportsTx gives the next page of unscoped imports the company of the
// mailbox they came from. The write is the one recordImportTx makes for a new
// import, so a replay finds nothing left to change.
func ScopeImportsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (scanned []ids.UUID, err error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `UPDATE capture_import i SET legal_entity_id = `+connectionEntitySQL+`
		FROM (SELECT i.id `+unscopedImportsSQL+` AND i.id > $1 ORDER BY i.id LIMIT $2) page
		WHERE i.id = page.id AND i.legal_entity_id IS NULL RETURNING i.id`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("capture: giving imports their mailbox's sending company: %w", err)
	}
	scanned, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("capture: giving imports their mailbox's sending company: %w", err)
	}
	return scanned, nil
}

// CountUnscopedImports counts imports ScopeImportsTx would still scope,
// reading at most maximum+1.
func CountUnscopedImports(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+unscopedImportsSQL+` LIMIT $1) unscoped`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("capture: counting imports without a sending company: %w", err)
	}
	return count, nil
}
