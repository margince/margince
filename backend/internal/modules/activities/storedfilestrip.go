// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StoredMessageFile is one captured file of a message whose bytes are still
// stored: the row, the provider's part ordinal, the object key and the bytes.
type StoredMessageFile struct {
	ID      ids.UUID
	Ordinal int
	Key     string
	Body    []byte
}

// StoredFilesOfMessageTx reads a captured message's stored files with their
// bytes, which a caller withholding them needs to find them in the original.
//
// Only files with an object behind them: a row already withheld has nothing to
// read. A file whose part ordinal is not one capture wrote is still returned,
// with ordinal 0, so its bytes are withheld even if no marker can name it.
func (s *Store) StoredFilesOfMessageTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]StoredMessageFile, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return nil, err
	}
	if s.blob == nil {
		return nil, ErrBlobstoreUnconfigured
	}
	rows, err := tx.Query(ctx, `
		SELECT id, storage_key, coalesce(external_part_id, '') FROM attachment
		 WHERE activity_id = $1 AND archived_at IS NULL
		   AND storage_key <> '' AND NOT bytes_withheld
		 ORDER BY id`, activityID)
	if err != nil {
		return nil, fmt.Errorf("activities: listing a message's stored files: %w", err)
	}
	var files []StoredMessageFile
	for rows.Next() {
		var f StoredMessageFile
		var partID string
		if err := rows.Scan(&f.ID, &f.Key, &partID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("activities: reading a message's stored file: %w", err)
		}
		if ordinal, err := strconv.Atoi(strings.TrimPrefix(partID, "part:")); err == nil {
			f.Ordinal = ordinal
		}
		files = append(files, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading a message's stored files: %w", err)
	}
	for i := range files {
		body, err := s.readObject(ctx, files[i].Key)
		if err != nil {
			return nil, err
		}
		files[i].Body = body
	}
	return files, nil
}

func (s *Store) readObject(ctx context.Context, key string) ([]byte, error) {
	rc, _, err := s.blob.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("activities: reading a stored file to withhold it: %w", err)
	}
	body, readErr := io.ReadAll(rc)
	if closeErr := rc.Close(); readErr == nil && closeErr != nil {
		readErr = closeErr
	}
	if readErr != nil {
		return nil, fmt.Errorf("activities: reading a stored file to withhold it: %w", readErr)
	}
	return body, nil
}

// WithholdStoredFilesTx turns stored files into withheld ones: each row keeps
// its name, size and type, loses its key and checksum and is marked
// bytes_withheld, with an audit row; then the objects are deleted.
//
// Objects last and inside the caller's transaction, as erasure does: a failed
// delete rolls the rows back with their keys intact, so no row is left naming
// an object that is gone, and a retry finds the same files again.
func (s *Store) WithholdStoredFilesTx(ctx context.Context, tx pgx.Tx, files []StoredMessageFile) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	if s.blob == nil {
		return ErrBlobstoreUnconfigured
	}
	for _, f := range files {
		tag, err := tx.Exec(ctx, `
			UPDATE attachment SET storage_key = '', checksum = NULL, bytes_withheld = true
			 WHERE id = $1 AND storage_key = $2 AND NOT bytes_withheld`, f.ID, f.Key)
		if err != nil {
			return fmt.Errorf("activities: withholding a stored file: %w", err)
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		if _, err := storekit.Audit(ctx, tx, "update", "attachment", f.ID,
			map[string]any{"bytes_withheld": false}, map[string]any{"bytes_withheld": true}); err != nil {
			return fmt.Errorf("activities: auditing a withheld file: %w", err)
		}
		if err := s.blob.Delete(ctx, f.Key); err != nil {
			return fmt.Errorf("activities: deleting a withheld file's object: %w", err)
		}
	}
	return nil
}
