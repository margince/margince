// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// restoreBudget bounds putting staged files back after the request is gone.
const restoreBudget = 30 * time.Second

// maxFilesPerMessage is the most staged files one message may name.
const maxFilesPerMessage = 20

// claimedUpload is a staged file taken off the staging table for one landing,
// kept whole so a landing that fails can put it back.
type claimedUpload struct {
	id, connection        ids.UUID
	filename, contentType string
	sha256                string
	size                  int64
	content               []byte
	createdAt             time.Time
}

// claimUploadsTx takes the caller's named uploads off staging, so two pushes
// naming one upload cannot both land it. The sizes are checked before any
// content is read.
func claimUploadsTx(ctx context.Context, tx pgx.Tx, user ids.UUID, uploads []ids.UUID) ([]extension.InboundFile, []claimedUpload, error) {
	if len(uploads) > maxFilesPerMessage {
		return nil, nil, invalid("a message carries at most %d files", maxFilesPerMessage)
	}
	distinct := make(map[ids.UUID]struct{}, len(uploads))
	for _, id := range uploads {
		distinct[id] = struct{}{}
	}
	if len(distinct) != len(uploads) {
		return nil, nil, invalid("a message names each file once")
	}
	if err := checkUploadsTx(ctx, tx, user, uploads); err != nil {
		return nil, nil, err
	}
	rows, err := tx.Query(ctx, `
		DELETE FROM personal_channel_upload WHERE id = ANY($1)
		RETURNING id, connection_id, filename, content_type, byte_size, sha256, content, created_at`, uploads)
	if err != nil {
		return nil, nil, err
	}
	byID := map[ids.UUID]claimedUpload{}
	for rows.Next() {
		var c claimedUpload
		if err := rows.Scan(&c.id, &c.connection, &c.filename, &c.contentType, &c.size, &c.sha256, &c.content, &c.createdAt); err != nil {
			rows.Close()
			return nil, nil, err
		}
		byID[c.id] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	files := make([]extension.InboundFile, 0, len(uploads))
	claimed := make([]claimedUpload, 0, len(uploads))
	for i, id := range uploads {
		c := byID[id]
		files = append(files, extension.InboundFile{Filename: c.filename, ContentType: c.contentType, Body: c.content, Ordinal: i})
		claimed = append(claimed, c)
	}
	return files, claimed, nil
}

// checkUploadsTx locks the named uploads and refuses one that is not the
// caller's, or a set larger than one message may carry.
func checkUploadsTx(ctx context.Context, tx pgx.Tx, user ids.UUID, uploads []ids.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT u.id, u.byte_size FROM personal_channel_upload u
		  JOIN personal_channel_connection c ON c.id = u.connection_id
		 WHERE c.user_id = $1 AND u.id = ANY($2)
		 FOR UPDATE OF u`, user, uploads)
	if err != nil {
		return err
	}
	found := map[ids.UUID]bool{}
	var total int64
	for rows.Next() {
		var id ids.UUID
		var size int64
		if err := rows.Scan(&id, &size); err != nil {
			rows.Close()
			return err
		}
		found[id] = true
		total += size
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range uploads {
		if !found[id] {
			return fmt.Errorf("upload %s is not one of yours: %w", id, apperrors.ErrNotFound)
		}
	}
	if total > extension.MaxInboundMessageBytes {
		return invalid("a message's files are %d bytes together, over the %d-byte limit", total, extension.MaxInboundMessageBytes)
	}
	return nil
}

// restoreUploads puts claimed uploads back on staging after a landing that kept
// nothing. The client can then send the message again with the same upload ids.
// It outlives the request: a client that hung up mid-landing must not lose the
// bytes it staged.
func restoreUploads(ctx context.Context, db *database.DB, claimed []claimedUpload) error {
	if len(claimed) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreBudget)
	defer cancel()
	return db.Tx(ctx, func(tx pgx.Tx) error {
		for _, c := range claimed {
			if _, err := tx.Exec(ctx, `
				INSERT INTO personal_channel_upload (id, connection_id, filename, content_type, byte_size, sha256, content, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (id) DO NOTHING`,
				c.id, c.connection, c.filename, c.contentType, c.size, c.sha256, c.content, c.createdAt); err != nil {
				return err
			}
		}
		return nil
	})
}
