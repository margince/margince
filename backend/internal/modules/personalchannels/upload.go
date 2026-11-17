// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// maxUploadBytes is the largest file one message may carry, the core's own
// per-file ceiling for captured files.
const maxUploadBytes = extension.MaxInboundFileBytes

// UploadBodyBytes is the request ceiling for one upload: the file and the
// multipart framing around it.
const UploadBodyBytes = maxUploadBytes + 64<<10

// uploadLifetime is how long a staged file waits for the message that names it.
const uploadLifetime = 24 * time.Hour

const uploadEntity = "personal_channel_upload"

// Upload is one staged file, as its sender learns it.
type Upload struct {
	ID          ids.UUID
	Filename    string
	ContentType string
	Size        int64
	SHA256      string
}

// StageUpload keeps one file for a message the caller will push next. The type
// is read from the bytes, never from the sender's claim.
func (s *Store) StageUpload(ctx context.Context, filename string, body io.Reader) (Upload, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionCreate); err != nil {
		return Upload{}, err
	}
	content, err := io.ReadAll(io.LimitReader(body, maxUploadBytes+1))
	if err != nil {
		return Upload{}, err
	}
	switch {
	case len(content) == 0:
		return Upload{}, fmt.Errorf("the file is empty: %w", apperrors.ErrInvalidArgument)
	case len(content) > maxUploadBytes:
		return Upload{}, fmt.Errorf("a file may be at most %d bytes: %w", maxUploadBytes, apperrors.ErrInvalidArgument)
	}
	sum := sha256.Sum256(content)
	up := Upload{
		Filename:    extension.SafeFilename(filename, 0),
		ContentType: extension.SniffContentType(content),
		Size:        int64(len(content)),
		SHA256:      hex.EncodeToString(sum[:]),
	}
	err = s.tx(ctx, func(tx pgx.Tx, user ids.UUID) error {
		conn, err := s.RequireEnabled(ctx, tx, user)
		if err != nil {
			return err
		}
		if err := underStagingLimit(ctx, tx, conn.ID, up.Size); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO personal_channel_upload (connection_id, filename, content_type, byte_size, sha256, content)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			conn.ID, up.Filename, up.ContentType, up.Size, up.SHA256, content).Scan(&up.ID); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "create", uploadEntity, up.ID, nil,
			map[string]any{"filename": up.Filename, "content_type": up.ContentType, "byte_size": up.Size})
		return err
	})
	return up, err
}

// The most one connection may hold staged at once, so a client in a loop cannot
// fill the database before the sweep runs.
const (
	maxStagedFiles = 50
	maxStagedBytes = 200 << 20
)

func underStagingLimit(ctx context.Context, tx pgx.Tx, connection ids.UUID, adding int64) error {
	var files int
	var bytes int64
	if err := tx.QueryRow(ctx, `SELECT count(*), coalesce(sum(byte_size), 0) FROM personal_channel_upload WHERE connection_id = $1`,
		connection).Scan(&files, &bytes); err != nil {
		return err
	}
	if files >= maxStagedFiles || bytes+adding > maxStagedBytes {
		return fmt.Errorf("at most %d files or %d MiB may wait staged; push the messages that name them first: %w",
			maxStagedFiles, maxStagedBytes>>20, apperrors.ErrConflict)
	}
	return nil
}

// SweepActor is the system principal the sweep job binds, and the only one
// Sweep admits.
const SweepActor = "personal_channel_sweep"

// onlyTheSweep refuses every caller but the sweep job: an open bulk delete would
// let anyone discard every member's staged files.
func onlyTheSweep(ctx context.Context) error {
	if p, ok := principal.Actor(ctx); ok && p.Type == principal.PrincipalSystem && p.ID == SweepActor {
		return nil
	}
	return fmt.Errorf("only the personal channel sweep deletes staged files: %w", apperrors.ErrPermissionDenied)
}

// Sweep deletes staged files no message named within uploadLifetime and
// expires replies no client acked within replyLifetime. Only the sweep job runs it.
func Sweep(ctx context.Context, db *database.DB, settler Settler, now time.Time) (files, replies int, err error) {
	if err := onlyTheSweep(ctx); err != nil {
		return 0, 0, err
	}
	if files, err = sweepUploads(ctx, db, now); err != nil {
		return files, 0, err
	}
	replies, err = expireReplies(ctx, db, settler, now)
	return files, replies, err
}

func sweepUploads(ctx context.Context, db *database.DB, now time.Time) (int, error) {
	var n int64
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM personal_channel_upload WHERE created_at < $1`, now.Add(-uploadLifetime))
		n = tag.RowsAffected()
		return err
	})
	return int(n), err
}
