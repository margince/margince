// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The system reads of mail. Mail lists the emails linked to one deal: only mail
// the whole workspace may see, and only whether it went out and whether anyone
// answered. The attachment reads run as the unit's service member and give a
// file only with a mail MailContent().Get would read. systemmail holds the SQL.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/systemmail"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

//nolint:ireturn // the published port, as Deals.
func (r *callRuntime) Activities() extension.Activities { return systemActivities{rt: r} }

type systemActivities struct{ rt *callRuntime }

func (a systemActivities) Mail(ctx context.Context, actAs extension.UserID, dealID string, q extension.MailQuery) (extension.Page[extension.MailSummary], error) {
	ctx, err := a.rt.systemRead(ctx, extension.SystemReadActivities)
	if err != nil {
		return extension.Page[extension.MailSummary]{}, err
	}
	deal, err := unitID(dealID, "deal")
	if err != nil {
		return extension.Page[extension.MailSummary]{}, err
	}
	member, err := unitID(string(actAs), "actAs")
	if err != nil {
		return extension.Page[extension.MailSummary]{}, err
	}
	query, err := systemmail.ParseQuery(q)
	if err != nil {
		return extension.Page[extension.MailSummary]{}, err
	}
	if err := a.rt.traceSystemRead(ctx, extension.SystemReadActivities, fieldmask.Deal, dealID); err != nil {
		return extension.Page[extension.MailSummary]{}, err
	}
	// The deal must open under the member's own grants, so a unit cannot read the
	// mail of a deal its actor could not see. A seat that is no colleague, or a
	// deal they cannot open, answers as a deal that does not exist.
	visible, err := dealShownTo(ctx, a.rt.deps.pool, member, deal, nil)
	if err != nil {
		return extension.Page[extension.MailSummary]{}, a.rt.systemReadFault(ctx, err)
	}
	if !visible {
		return extension.Page[extension.MailSummary]{}, extension.ErrNotFound
	}
	page, err := systemmail.ReadDeal(ctx, a.rt.deps.pool, deal, query)
	if err != nil && !errors.Is(err, extension.ErrNotFound) {
		return extension.Page[extension.MailSummary]{}, a.rt.systemReadFault(ctx, err)
	}
	return page, err
}

func (a systemActivities) Attachments(ctx context.Context, actAs extension.UserID, mailID string) ([]extension.MailAttachment, error) {
	seatCtx, seat, err := a.rt.seatRead(ctx, extension.SystemReadMailAttachments, actAs)
	if err != nil {
		return nil, err
	}
	var out []extension.MailAttachment
	err = seat.fileRead(seatCtx, "activity", mailID, func(tx pgx.Tx, files systemmail.Files, mail ids.UUID) (err error) {
		out, err = files.List(seatCtx, tx, mail)
		return err
	})
	return out, err
}

func (a systemActivities) OpenAttachment(ctx context.Context, actAs extension.UserID, attachmentID string) (extension.MailAttachmentFile, error) {
	seatCtx, seat, err := a.rt.seatRead(ctx, extension.SystemReadMailAttachments, actAs)
	if err != nil {
		return extension.MailAttachmentFile{}, err
	}
	var (
		files systemmail.Files
		found extension.MailAttachment
		key   string
	)
	err = seat.fileRead(seatCtx, "attachment", attachmentID, func(tx pgx.Tx, f systemmail.Files, id ids.UUID) (err error) {
		files = f
		found, key, err = f.Open(seatCtx, tx, id)
		return err
	})
	if err != nil {
		return extension.MailAttachmentFile{}, err
	}
	// Read after the transaction, so a slow object store holds no connection.
	file, err := files.Read(seatCtx, found, key)
	if err != nil {
		return extension.MailAttachmentFile{}, seat.fileFault(err)
	}
	return file, nil
}

// fileRead traces one attachment read and runs it in a transaction of the seat.
func (s systemSeat) fileRead(ctx context.Context, recordType, raw string, read func(pgx.Tx, systemmail.Files, ids.UUID) error) error {
	id, err := unitID(raw, recordType)
	if err != nil {
		return err
	}
	if err := s.trace(recordType, raw, nil); err != nil {
		return err
	}
	files := systemmail.Files{Unit: s.rt.unit, ActAs: s.member, Blob: s.rt.deps.blob}
	if err := database.WithWorkspaceTx(ctx, s.rt.deps.pool, func(tx pgx.Tx) error { return read(tx, files, id) }); err != nil {
		return s.fileFault(err)
	}
	return nil
}

// fileFault passes a published refusal on, and answers any other fault as the
// core's own failure.
func (s systemSeat) fileFault(err error) error {
	if published := systemmail.Published(err); published != nil {
		return published
	}
	return s.seatFault(err)
}
