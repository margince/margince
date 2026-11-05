// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// documentLinksDataTask records the document links of mail captured before link
// rows existed, from the stored original. It writes through the same path as a
// live capture, so a link a human archived is not touched. A mail whose original
// has expired under the raw-capture retention window is out of reach.
func documentLinksDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792860002_document_links_from_stored_mail", Version: 1,
		Title: "Record document links sent in captured mail", Requires: []string{"core/1792896283"},
		Steps: []datatask.Step{{
			Name: "record_document_links", Limit: 50, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateMailRecipientOriginals, Batch: recordDocumentLinksBatch,
		}},
	}
}

type storedMailForLinks struct {
	ID         ids.UUID
	System     string
	SourceID   string
	CapturedBy string
	Payload    []byte
}

func recordDocumentLinksBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.source_system, a.source_id, a.captured_by, rc.payload `+
		mailRecipientOriginalScope+` AND a.id > @scan_after ORDER BY a.id LIMIT @scan_limit FOR UPDATE OF a`,
		pgx.NamedArgs{"scan_after": after, "scan_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	mails, err := pgx.CollectRows(rows, pgx.RowToStructByPos[storedMailForLinks])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(mails))
	for _, mail := range mails {
		scanned = append(scanned, mail.ID)
	}
	before, err := mailsWithLinks(ctx, tx, scanned)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	keeper := capturedFileKeeper{store: activities.NewStore(nil)}
	for _, mail := range mails {
		// An unreadable original is an outcome of the run. The batch moves past it.
		if err := recordStoredMailLinks(ctx, tx, keeper, mail); err != nil && !errors.Is(err, errUnreadableOriginal) {
			return datatask.BatchOut{}, err
		}
	}
	after2, err := mailsWithLinks(ctx, tx, scanned)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, after2-before)
}

var errUnreadableOriginal = errors.New("compose: a stored mail original could not be read")

// recordStoredMailLinks reads one stored original and records its links, or
// answers errUnreadableOriginal.
func recordStoredMailLinks(ctx context.Context, tx pgx.Tx, keeper capture.FileKeeper, mail storedMailForLinks) error {
	raw, err := capture.DecodeStoredOriginal(mail.Payload)
	if err != nil {
		return fmt.Errorf("%w: %w", errUnreadableOriginal, err)
	}
	parsed, err := mailmap.Parse(raw, "")
	if err != nil {
		return fmt.Errorf("%w: %w", errUnreadableOriginal, err)
	}
	fields, ok := parsed.ToRecord("gmail", nil).Fields.(capture.ActivityFields)
	if !ok {
		return apperrors.ErrInvalidArgument
	}
	from := capture.FileSource{System: mail.System, MessageID: mail.SourceID, CapturedBy: mail.CapturedBy}
	_, err = capture.RecordDocumentLinksTx(ctx, tx, keeper, ids.From[ids.ActivityKind](mail.ID), from, fields.Envelope)
	return err
}

// mailsWithLinks counts mails, not rows, so a batch's Changed never exceeds Scanned.
func mailsWithLinks(ctx context.Context, tx pgx.Tx, mails []ids.UUID) (int, error) {
	var n int
	err := tx.QueryRow(ctx,
		`SELECT count(DISTINCT activity_id) FROM attachment WHERE category = 'link' AND activity_id = ANY(@ids)`,
		pgx.NamedArgs{"ids": mails}).Scan(&n)
	return n, err
}
