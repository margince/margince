// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// One object per distinct file. A captured file whose bytes an attachment row
// already holds points at that row's object instead of storing a second copy.
//
// Keys stay random rather than named after the checksum. The stored-object
// ledger keeps a reaped key's tombstone forever, so a checksum key could not be
// stored again once its last copy was reaped.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedCopy identifies a file's bytes: the checksum, and the size so a
// collision would also have to match in length.
type storedCopy struct {
	checksum string
	size     int
}

// ErrReusedFileGone is a capture that meant to point at an object another row
// held, after that row lost it to an erasure or a strip. The sync retries, and
// the retry stores the bytes fresh.
var ErrReusedFileGone = errors.New("a stored copy of this file was removed while the message was captured; the next sync stores it again")

// storedCopies answers, for each distinct file, the key of an object a row
// already holds with those bytes, under this workspace's own prefix.
func (s *Store) storedCopies(ctx context.Context, workspace ids.WorkspaceID, files []CapturedFile) (map[storedCopy]string, error) {
	sums := make([]string, 0, len(files))
	for _, file := range files {
		sum := sha256.Sum256(file.Body)
		sums = append(sums, hex.EncodeToString(sum[:]))
	}
	held := make(map[storedCopy]string, len(files))
	// Bound to the context's workspace, as the intent ledger is: the capture
	// path reaches this from an installation-wide handle.
	err := s.db.ForWorkspace(workspace).Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT ON (checksum, byte_size) checksum, byte_size, storage_key
			  FROM attachment
			 WHERE checksum = ANY($1::text[]) AND NOT bytes_withheld
			   AND starts_with(storage_key, $2)
			 ORDER BY checksum, byte_size, created_at`, sums, blobstore.WorkspaceKey(workspace, string(storedobjects.KindAttachment), ""))
		if err != nil {
			return fmt.Errorf("find stored copies of captured files: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var copyOf storedCopy
			var key string
			if err := rows.Scan(&copyOf.checksum, &copyOf.size, &key); err != nil {
				return fmt.Errorf("read a stored copy: %w", err)
			}
			held[copyOf] = key
		}
		return rows.Err()
	})
	return held, err
}

// holdReusedObjectTx locks a row that still holds a reused file's object. An
// erasure or strip of that row then waits for the new row to commit, sees two
// rows naming the object, and keeps the bytes.
func holdReusedObjectTx(ctx context.Context, tx pgx.Tx, file StagedFile) error {
	if !file.reused {
		return nil
	}
	var held int
	err := tx.QueryRow(ctx, `
		SELECT 1 FROM attachment
		 WHERE storage_key = $1 AND NOT bytes_withheld
		 LIMIT 1 FOR SHARE`, file.key).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrReusedFileGone
	}
	if err != nil {
		return fmt.Errorf("hold a stored copy of a captured file: %w", err)
	}
	return nil
}
