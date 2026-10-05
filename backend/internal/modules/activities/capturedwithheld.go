// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ErrBytesWithheld is a file recorded by name only: capture kept no bytes for
// it (RecordWithheldFiles), so there is nothing to serve or read. It answers as
// not-found, the contract's only refusal for a file that cannot be had.
var ErrBytesWithheld = fmt.Errorf("activities: the file's bytes were not kept: %w", apperrors.ErrNotFound)

// WithheldFile is one file a private message carried whose bytes capture did
// not keep: what arrived, without the arrival itself.
type WithheldFile struct {
	PartID       string
	Filename     string
	ContentType  string
	DeclaredType string
	ByteSize     int
}

// RecordWithheldFiles writes an attachment row for each file a private message
// carried, with no stored object behind it, so the mailbox owner still sees
// what was attached.
//
// The row carries bytes_withheld, an empty storage key and no checksum. Every
// reader that would fetch bytes refuses it, and it is never rolled up to an
// account: a file nobody can open has no place in a company's library.
func (s *Store) RecordWithheldFiles(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from CapturedFileSource, files []WithheldFile,
) error {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	if err := refuseUnderivedCategory(from); err != nil {
		return err
	}
	for _, file := range files {
		row := capturedFileRow{
			id: ids.NewV7(),
			file: CapturedFile{
				PartID: file.PartID, Filename: file.Filename,
				ContentType: file.ContentType, DeclaredType: file.DeclaredType,
			},
			byteSize: file.ByteSize,
			withheld: true,
		}
		if err := insertCapturedAttachment(ctx, tx, activityID, nil, from, row); err != nil {
			return err
		}
	}
	return nil
}

// refuseWithheldBytes answers ErrBytesWithheld for a file with no bytes, so a
// reader that would fetch them stops at the row rather than at an empty key.
func refuseWithheldBytes(ctx context.Context, tx pgx.Tx, attachmentID ids.UUID) error {
	var withheld bool
	if err := tx.QueryRow(ctx,
		`SELECT bytes_withheld FROM attachment WHERE id = $1`, attachmentID).Scan(&withheld); err != nil {
		return fmt.Errorf("activities: reading whether a file's bytes were kept: %w", err)
	}
	if withheld {
		return ErrBytesWithheld
	}
	return nil
}
