// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ErrBytesWithheld is a file recorded by name only: capture kept no bytes for
// it (RecordWithheldFiles), so there is nothing to serve or read. It answers as
// not-found, the contract's only refusal for a file that cannot be had.
var ErrBytesWithheld = fmt.Errorf("activities: the file's bytes were not kept: %w", apperrors.ErrNotFound)

// fieldBytesWithheld is the column, and the audit images' key, that marks a
// file recorded by name only.
const fieldBytesWithheld = "bytes_withheld"

// WithheldFile is one file a private message carried whose bytes capture did
// not keep: what arrived, without the arrival itself.
type WithheldFile struct {
	PartID       string
	Filename     string
	ContentType  string
	DeclaredType string
	ByteSize     int
	// Checksum fingerprints the bytes that were not kept. Two readers use it.
	// The replay proof compares an arriving message's parts with the stored
	// rows. AdoptWithheldFiles refuses bytes whose fingerprint is not the one
	// this row was named under. Empty for a private thread, whose row states
	// nothing about its contents.
	Checksum string
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
			checksum: nullIfEmpty(file.Checksum),
			withheld: true,
		}
		if err := insertCapturedAttachment(ctx, tx, activityID, from, row); err != nil {
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
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("activities: reading whether a file's bytes were kept: %w", err)
	}
	if withheld {
		return ErrBytesWithheld
	}
	return nil
}

// AdoptWithheldFiles gives rows that name a file without bytes the bytes a
// later capture carried, and reports how many it gave.
//
// One message reaching a copied seat and an addressed seat arrives twice. The
// copied arrival withholds its bytes; the addressed one carries them. Files are
// staged for a new activity only, so without this the addressed seat joins the
// existing row and never receives them.
func (s *Store) AdoptWithheldFiles(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from CapturedFileSource, files []CapturedFile,
) (int, error) {
	// Create, not update, and the row existing does not change that. What is
	// recorded is a file arriving with a message. That is the act the capture
	// principal is admitted for, as StageCapturedFiles argues too. No seat's
	// own edit passes here, so update would name an unused authority.
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, nil
	}
	// Staged only where a row is still waiting for bytes. The ordinary replay of
	// a message that already has them stages nothing, so a join never leaves an
	// object no row points at.
	waiting, err := s.partsAwaitingBytes(ctx, tx, from)
	if err != nil || len(waiting) == 0 {
		return 0, err
	}
	// The fingerprint has to match the one the row was named under. On the part
	// id alone, a later message with different bytes at the same ordinal would
	// replace the file this row names. The row would still read as the
	// original.
	carried := make([]CapturedFile, 0, len(files))
	for _, file := range files {
		named, waits := waiting[file.PartID]
		if !waits {
			continue
		}
		sum := sha256.Sum256(file.Body)
		if named != hex.EncodeToString(sum[:]) {
			continue
		}
		carried = append(carried, file)
	}
	staged, err := s.StageCapturedFiles(ctx, carried)
	if err != nil {
		return 0, err
	}
	return s.giveWithheldBytes(ctx, tx, activityID, from, staged)
}

// partsAwaitingBytes names this message's parts whose row has none yet, each
// with the fingerprint it was named under.
//
// The part id is derived from a part's ordinal, so it says where a file sat in
// the message and not which file it was. The fingerprint is what says that.
func (s *Store) partsAwaitingBytes(
	ctx context.Context, tx pgx.Tx, from CapturedFileSource,
) (map[string]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT external_part_id, coalesce(checksum, '')
		  FROM attachment
		 WHERE external_source_id = $1 AND bytes_withheld`,
		providerMessageKey(from))
	if err != nil {
		return nil, fmt.Errorf("read the files still awaiting bytes: %w", err)
	}
	defer rows.Close()
	waiting := map[string]string{}
	for rows.Next() {
		var part, sum string
		if err := rows.Scan(&part, &sum); err != nil {
			return nil, fmt.Errorf("read a file awaiting bytes: %w", err)
		}
		waiting[part] = sum
	}
	return waiting, rows.Err()
}

// giveWithheldBytes points each waiting row at the object now holding its file.
//
// The guard on bytes_withheld makes a repeat a no-op rather than a second
// object under a key the row already points at.
func (s *Store) giveWithheldBytes(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from CapturedFileSource, staged []StagedFile,
) (int, error) {
	adopted := 0
	for _, file := range staged {
		var id ids.UUID
		var size int
		// byte_size is left alone: the withheld row recorded the sender's size
		// when it named the file, and these are that file's bytes.
		err := tx.QueryRow(
			ctx, `
			UPDATE attachment
			   SET storage_key = $3, checksum = $4, bytes_withheld = false
			 WHERE external_source_id = $1 AND external_part_id = $2
			   AND bytes_withheld
			RETURNING id, byte_size`,
			providerMessageKey(from), file.file.PartID, file.key, file.checksum,
		).Scan(&id, &size)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return adopted, fmt.Errorf("give a withheld file its bytes: %w", err)
		}
		// The row's own image builder, so the before and after cannot drift from
		// the one the create path audits.
		row := capturedFileRow{byteSize: size, withheld: true}
		before := row.auditImage(activityID, from)
		row.withheld = false
		if _, err := storekit.Audit(
			ctx, tx, "update", "attachment", id, before, row.auditImage(activityID, from),
		); err != nil {
			return adopted, fmt.Errorf("audit a file given its bytes: %w", err)
		}
		adopted++
	}
	return adopted, nil
}
