// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// withheldCopiedParts is the breadcrumb a copied-only message leaves.
//
// Separate from the private one because the reasons differ. A private thread
// withholds because somebody judged it personal. This withholds because nobody
// here was addressed, which no human decided and anybody may undo by being put
// in To.
const withheldCopiedParts = "capture_copied_parts_withheld"

// copiedOnlyTx reports whether no seat this installation owns sent this mail or
// stood in its To line.
//
// Three states keep their bytes. Reading any of them as copied-only costs a
// reader a file they expect to open:
//
//   - an outbound message, which a seat here sent, whatever its To line says
//   - RecipientsUnreadable, where a To header would not parse, so the reader's
//     own address may be missing from what did
//   - no open recipients at all, which is undisclosed delivery rather than
//     evidence that this mailbox was merely copied
//
// Each of those stores the bytes. Withholding costs a file; storing costs disk.
func copiedOnlyTx(ctx context.Context, tx pgx.Tx, fields ActivityFields) (bool, error) {
	if fields.Kind != kindEmail || fields.Envelope == nil {
		return false, nil
	}
	if fields.Direction == connector.DirectionOutbound {
		return false, nil
	}
	envelope := fields.Envelope
	if envelope.RecipientsUnreadable || len(envelope.OpenRecipients) == 0 {
		return false, nil
	}
	own, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return false, err
	}
	return !slices.ContainsFunc(envelope.DirectRecipients, own.Covers), nil
}

// copiedPartsWithheld records that a copied-only message's files were kept out,
// with the count and no filename, type or size.
//
// The sender controls all three and system_log is operational, so the count is
// ours to state and the names are not.
func (s *Sink) copiedPartsWithheld(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, count int,
) error {
	if count == 0 {
		return nil
	}
	return s.logBreadcrumbTx(ctx, tx, withheldCopiedParts, rec,
		"no seat here was addressed",
		map[string]any{fieldDroppedParts: count})
}

// adoptWithheldParts gives a message another mailbox captured first the bytes
// this arrival carries.
//
// Reached when a seat joins an existing activity. A copied seat withheld the
// bytes, and the join is the only point where they can still arrive.
//
// A private thread keeps withholding, whoever arrives: being addressed is not
// permission to store what somebody judged personal.
func (s *Sink) adoptWithheldParts(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	rec connector.NormalizedRecord, fields ActivityFields,
) error {
	if s.files == nil || len(rec.Parts) == 0 {
		return nil
	}
	private, _, err := messageIsPrivateTx(ctx, tx, rec, fields)
	if err != nil || private {
		return err
	}
	copied, err := copiedOnlyTx(ctx, tx, fields)
	if err != nil || copied {
		return err
	}
	_, err = s.files.AdoptWithheld(
		ctx, tx, activityID, fileSourceFor(rec, fields), capturedFilesOf(rec.Parts),
	)
	return err
}
