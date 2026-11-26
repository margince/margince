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

	"github.com/margince/margince/backend/internal/compose/systemmail"
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
	files, mail, err := seat.files("activity", mailID)
	if err != nil {
		return nil, err
	}
	out, err := files.List(seatCtx, mail)
	return out, seat.fileFault(err)
}

func (a systemActivities) OpenAttachment(ctx context.Context, actAs extension.UserID, attachmentID string) (extension.MailAttachmentFile, error) {
	seatCtx, seat, err := a.rt.seatRead(ctx, extension.SystemReadMailAttachments, actAs)
	if err != nil {
		return extension.MailAttachmentFile{}, err
	}
	files, id, err := seat.files("attachment", attachmentID)
	if err != nil {
		return extension.MailAttachmentFile{}, err
	}
	file, err := files.Open(seatCtx, id)
	return file, seat.fileFault(err)
}

// files reads the id of one attachment read, writes its trace, and answers the
// reader bound to the seat.
func (s systemSeat) files(recordType, raw string) (systemmail.Files, ids.UUID, error) {
	id, err := unitID(raw, recordType)
	if err != nil {
		return systemmail.Files{}, ids.Nil, err
	}
	if err := s.trace(recordType, raw, nil); err != nil {
		return systemmail.Files{}, ids.Nil, err
	}
	return systemmail.Files{Unit: s.rt.unit, ActAs: s.member, Blob: s.rt.deps.blob, Pool: s.rt.deps.pool}, id, nil
}

// fileFault passes a published refusal on, and answers any other fault as the
// core's own failure.
func (s systemSeat) fileFault(err error) error {
	if err == nil {
		return nil
	}
	if published := systemmail.Published(err); published != nil {
		return published
	}
	return s.seatFault(err)
}
