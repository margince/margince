// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordfiles"
)

func (s *Store) PreviewFileCarryTx(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]recordfiles.CarriedFile, error) {
	if err := auth.Require(ctx, kind, principal.ActionRead); err != nil {
		return nil, err
	}
	for _, id := range []ids.UUID{source, target} {
		if err := ensureAttachmentParentVisible(ctx, tx, kind, id); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT `+attachmentColumns+`, at.archived_at, at.updated_at
 FROM attachment at WHERE at.entity_type = @kind AND at.entity_id = @source ORDER BY at.id LIMIT 501`, pgx.StrictNamedArgs{"kind": kind, "source": source})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []recordfiles.CarriedFile{}
	for rows.Next() {
		var columns attachmentScan
		var file recordfiles.CarriedFile
		if err := rows.Scan(append(columns.targets(), &file.ArchivedAt, &file.UpdatedAt)...); err != nil {
			return nil, err
		}
		attachment := columns.attachment()
		file.ID, file.Filename, file.Title = ids.UUID(attachment.Id), attachment.Filename, attachment.Title
		file.ContentType, file.ByteSize, file.Checksum = attachment.ContentType, attachment.ByteSize, attachment.Checksum
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(files) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return files, nil
}
