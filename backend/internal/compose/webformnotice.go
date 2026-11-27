// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// tellWebsiteLeadOwner uses the record's final owner, never the form's fallback
// or the coordinator who only holds a task about an unassigned inquiry.
func tellWebsiteLeadOwner(ctx context.Context, db *database.DB, formID, source ids.UUID, target notices.Target) error {
	form, err := webforms.NewStore(db, slog.Default()).GetForm(ctx, formID)
	if err != nil {
		return err
	}
	if !form.NotifyOwner {
		return nil
	}
	return database.WithWorkspaceWriteTx(ctx, db.Pool(), func(bound context.Context) error {
		owner, err := websiteNoticeOwner(bound, db, target)
		if err != nil || owner == nil {
			return err
		}
		seat, live, err := seatContext(bound, identity.NewService(db.Pool()), noticeWorkspace(bound), *owner)
		if err != nil || !live {
			return err
		}
		current, err := websiteNoticeOwner(seat, db, target)
		if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		if err != nil || current == nil || *current != *owner {
			return err
		}
		note, err := activities.NewStore(db).GetActivity(seat, ids.From[ids.ActivityKind](source), storekit.LiveOnly)
		if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if note.WebForm == nil {
			return nil
		}
		contactName, company := "", ""
		if note.WebForm.FullName != nil {
			contactName = *note.WebForm.FullName
		}
		if note.WebForm.Company != nil {
			company = *note.WebForm.Company
		}
		subject := fmt.Sprintf(systemtext.Copy("form.owner_subject", textlang.English), mailcopy.OneLine(form.Name))
		body := fmt.Sprintf(systemtext.Copy("form.owner_body", textlang.English), mailcopy.OneLine(contactName), mailcopy.OneLine(company), mailcopy.OneLine(note.WebForm.Email))
		message := &systemtext.Message{Subject: systemtext.Sentence("form.owner_subject", systemtext.Label(form.Name)), Body: systemtext.Sentence("form.owner_body", systemtext.Label(contactName), systemtext.Label(company), systemtext.Label(note.WebForm.Email))}
		return db.Tx(bound, func(tx pgx.Tx) error {
			noticeID, fresh, err := notices.NewStore(db).CreateTxNew(bound, tx, notices.NewNotice{Recipient: ids.From[ids.UserKind](*owner), Kind: notices.KindWebsiteLead, Subject: subject, Body: body, SystemText: message, Target: target, DedupeKey: "website_lead:" + source.String()})
			if err != nil || !fresh {
				return err
			}
			delivery, err := notices.DeliveryFor(bound, tx, ids.From[ids.UserKind](*owner), notices.ClassWebsiteLead)
			if err != nil || delivery != notices.DeliveryEmail {
				return err
			}
			queue, err := jobs.NewTxInserter(slog.Default())
			if err != nil {
				return err
			}
			return queue.EnqueueTx(bound, tx, SendNotificationEmailArgs{Workspace: noticeWorkspace(bound), NoticeID: noticeID.String()}, notificationMailOpts())
		})
	})
}

// websiteNoticeOwner reads ownership through the same scoped record readers
// used by the API. A notice cannot give a seat facts their record hides.
func websiteNoticeOwner(ctx context.Context, db *database.DB, target notices.Target) (*ids.UUID, error) {
	store := newContactsStore(db)
	switch target.Type {
	case "lead":
		lead, err := store.GetLead(ctx, ids.From[ids.LeadKind](target.ID), storekit.LiveOnly)
		if err != nil || lead.OwnerId == nil {
			return nil, err
		}
		return new(ids.UUID(*lead.OwnerId)), nil
	case "contact":
		contact, err := store.GetContact(ctx, ids.From[ids.ContactKind](target.ID), storekit.LiveOnly)
		if err != nil || contact.OwnerId == nil {
			return nil, err
		}
		return new(ids.UUID(*contact.OwnerId)), nil
	default:
		return nil, fmt.Errorf("website lead notice has unsupported target %q", target.Type)
	}
}

// websiteNoticeStillOwned prevents a queued email reaching a former owner.
func websiteNoticeStillOwned(ctx context.Context, db *database.DB, target notices.Target, recipient ids.UUID) (bool, error) {
	owner, err := websiteNoticeOwner(ctx, db, target)
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	return err == nil && owner != nil && *owner == recipient, err
}
