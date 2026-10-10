// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// What to do when the document already holding a set of bytes failed to read.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// discardFailedDocument removes a document whose ingest failed, so the same
// bytes can be filed again.
//
// A failed ingest keeps the row and deletes the stored object, and the partial
// unique index on (corpus_id, checksum) covers live rows.
//
// So the row refused the re-upload its own advice asks for. Nothing cites it,
// because a terminally failed ingest leaves no passages.
func discardFailedDocument(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// One statement, so the row it audits is the row it removed.
	//
	// A separate read would describe a row another transaction could have
	// changed, and the delete takes the lock that read would have asked for.
	var filename, checksum string
	switch err := tx.QueryRow(ctx,
		`DELETE FROM knowledge_document
		  WHERE id = $1 AND archived_at IS NULL AND ingest_status = 'failed'
		  RETURNING filename, checksum`, id).Scan(&filename, &checksum); {
	case err == nil:
	case errors.Is(err, pgx.ErrNoRows):
		// A concurrent upload of the same bytes got here first, so there is
		// nothing to discard. The insert that follows meets its live row and
		// answers already-filed, where refusing here would answer not-found
		// for an upload.
		return nil
	default:
		return fmt.Errorf("discard the failed document to replace it: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "delete", "knowledge_document", id, map[string]any{
		filenameKey: filename,
		checksumKey: checksum,
	}, nil); err != nil {
		return fmt.Errorf("audit the failed corpus document's replacement: %w", err)
	}
	return nil
}
