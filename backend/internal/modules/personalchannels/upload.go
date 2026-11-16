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

// ReadUploads returns the caller's own staged files by id, every one or none:
// an id that is not theirs is not found, as one that never existed.
func (s *Store) ReadUploads(ctx context.Context, uploads []ids.UUID) ([]extension.InboundFile, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []extension.InboundFile
	err := s.tx(ctx, func(tx pgx.Tx, user ids.UUID) error {
		var err error
		out, err = readUploadsTx(ctx, tx, user, uploads)
		return err
	})
	return out, err
}

func readUploadsTx(ctx context.Context, tx pgx.Tx, user ids.UUID, uploads []ids.UUID) ([]extension.InboundFile, error) {
	rows, err := tx.Query(ctx, `
		SELECT u.id, u.filename, u.content_type, u.content
		  FROM personal_channel_upload u
		  JOIN personal_channel_connection c ON c.id = u.connection_id
		 WHERE c.user_id = $1 AND u.id = ANY($2)`, user, uploads)
	if err != nil {
		return nil, err
	}
	byID := map[ids.UUID]extension.InboundFile{}
	for rows.Next() {
		var id ids.UUID
		var f extension.InboundFile
		if err := rows.Scan(&id, &f.Filename, &f.ContentType, &f.Body); err != nil {
			rows.Close()
			return nil, err
		}
		byID[id] = f
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]extension.InboundFile, 0, len(uploads))
	for i, id := range uploads {
		f, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("upload %s is not one of yours: %w", id, apperrors.ErrNotFound)
		}
		f.Ordinal = i
		out = append(out, f)
	}
	return out, nil
}

// SweepActor is the system principal the sweep job binds, and the only one
// SweepUploads admits.
const SweepActor = "personal_channel_sweep"

// onlyTheSweep refuses every caller but the sweep job: an open bulk delete would
// let anyone discard every member's staged files.
func onlyTheSweep(ctx context.Context) error {
	if p, ok := principal.Actor(ctx); ok && p.Type == principal.PrincipalSystem && p.ID == SweepActor {
		return nil
	}
	return fmt.Errorf("only the personal channel sweep deletes staged files: %w", apperrors.ErrPermissionDenied)
}

// SweepUploads deletes staged files no message named within uploadLifetime.
func (s *Store) SweepUploads(ctx context.Context, now time.Time) (int, error) {
	if err := onlyTheSweep(ctx); err != nil {
		return 0, err
	}
	var n int64
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM personal_channel_upload WHERE created_at < $1`, now.Add(-uploadLifetime))
		n = tag.RowsAffected()
		return err
	})
	return int(n), err
}
