// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Asking whether a message may be sent, before there is a message.
//
// The per-contact guard answers "may we write to this contact, per purpose". That
// is the right question for a record page and the wrong one for a composer: the
// engine resolves a category from the THREAD a message answers, the deal or
// invoice it names and the evidence the sender offers, none of which belong to
// a contact in the abstract. So a composer that only had the guard would show a
// verdict about a different question than the one the send asks.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// errNoPreviewAuthority is the composition defect of a preview surface built
// with no engine behind it. No sentinel, for the same reason
// errNoDeliveryStager carries none: it is a wiring fault and must surface as
// the 500 it is rather than borrow a refusal that would tell the caller
// something untrue about their request.
var errNoPreviewAuthority = errors.New("activities: preview has no consent authority wired")

// SendPreviewer answers what the engine would decide, and records nothing.
//
// Injected because consent owns the engine and a module never imports a
// sibling. The consent gate's own Preview satisfies it.
type SendPreviewer interface {
	decisionPreviewer
	// PreviewTx answers on the caller's transaction, so it sees a contact the
	// preview filed there and rolls back.
	PreviewTx(ctx context.Context, tx pgx.Tx, req commsauthz.Request) (commsauthz.DecisionSet, error)
}

// decisionPreviewer is the half of SendPreviewer a send asks while it
// prepares, when no transaction of its own is open.
type decisionPreviewer interface {
	Preview(ctx context.Context, req commsauthz.Request) (commsauthz.DecisionSet, error)
}

// SendPreview is the engine's answer, and which addresses the send would file
// as new contacts before asking it.
type SendPreview struct {
	Decisions commsauthz.DecisionSet
	// NewContacts are the folded addresses no record holds. The decisions about
	// them are the ones the contact the send makes will get.
	NewContacts      map[string]bool
	readableSubjects map[previewSubject]bool
	// ContentFindings are the content rules an advertising draft does not
	// meet. Advice only: none of them changes a decision.
	ContentFindings []DraftContentFinding
	// MailboxUnavailable is true when the caller holds no mailbox that can
	// transmit, the refusal the send itself would give.
	MailboxUnavailable bool
}

// PreviewSendInput is a message a composer has not written yet.
//
// Subject and Body reach the decision as the authored text staging reads for
// an offer, so the preview answers what the send will. They are also checked
// against the advertising content rules.
type PreviewSendInput struct {
	Subject  string
	Body     string
	HTMLBody string
	// AttachmentIDs are the files the message will carry. Their names are read
	// for an offer as the send reads them.
	AttachmentIDs []ids.UUID
	Recipients    []string
	Context       commsauthz.Category
	// LegacyPurposeKey is carried for one reason: the SEND still consults it
	// where the record supports no category on its own, so a preview that
	// could not pass it would answer a different question than the send it
	// previews — and then disagree with it, which is the failure this endpoint
	// exists to prevent.
	LegacyPurposeKey string
	MarketingPurpose string
	OperatorReason   string
	Evidence         commsauthz.Evidence
}

// PreviewSend answers what the engine would decide for this message.
//
// The origin resolves FIRST, exactly as a real send resolves it: a caller who
// names an anchor or a record they cannot read gets the row-scope answer here,
// before anything is asked about anybody's consent. Without that a caller could
// name a stranger's deal and have the engine answer about it — an unauthorized
// read wearing a preview.
//
// It runs on the store's own transaction through the previewer, which rolls
// back. Nothing is recorded: no decision rows, and no lawful basis, because a
// basis is the ground a SEND relies on and nothing has been sent.
func (s *Store) PreviewSend(ctx context.Context, origin SendOrigin, in PreviewSendInput, previewer SendPreviewer) (SendPreview, error) {
	// THE SAME GRANT THE SEND NEEDS, and asked first.
	//
	// A preview answers about a message this caller could send, so a caller who
	// could not send must not be able to ask. Without it the endpoint is a
	// consent oracle for anyone with a session: name an address, learn whether
	// that contact has objected. The record probe below is not a substitute — it
	// gates the RECORDS named, and a reply preview names none.
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return SendPreview{}, err
	}
	if previewer == nil {
		// A preview with no authority behind it would answer "allowed" about a
		// question nobody asked. Refused the way the send path refuses a
		// missing gate: absence of the engine never reads as permission.
		return SendPreview{}, errNoPreviewAuthority
	}
	links, err := origin.Resolve(ctx, s)
	if err != nil {
		return SendPreview{}, err
	}
	files, err := s.PreviewSendAttachments(ctx, in.AttachmentIDs)
	if err != nil {
		return SendPreview{}, err
	}
	// The question the send asks, from the send's own builder.
	req := outboundMessage{in: in.asSend(), links: links, files: files}.authorization(origin)
	var preview SendPreview
	if origin.isReply() || s.recipientFiler == nil {
		preview.Decisions, err = previewer.Preview(ctx, req)
	} else {
		preview, err = s.previewFilingNewAddresses(ctx, in.Recipients, req, previewer)
	}
	if err != nil {
		return SendPreview{}, err
	}
	if preview.Decisions, err = s.assessPreviewFooter(ctx, in, preview.Decisions); err != nil {
		return SendPreview{}, err
	}
	if preview.ContentFindings, err = s.draftContentFindings(ctx, in, preview.Decisions); err != nil {
		return SendPreview{}, err
	}
	if preview.MailboxUnavailable, err = s.mailboxUnavailable(ctx); err != nil {
		return SendPreview{}, err
	}
	return s.withReadablePreviewSubjects(ctx, preview)
}

// mailboxUnavailable asks the question the send asks. Only the refusal that
// names a missing mailbox is an answer. Any other error is a failed question.
func (s *Store) mailboxUnavailable(ctx context.Context) (bool, error) {
	_, err := s.sendableMailProvider(ctx)
	if _, none := errors.AsType[*MailboxNotSendCapableError](err); none {
		return true, nil
	}
	return false, err
}

// errPreviewFiled unwinds the preview's transaction once it has its answer:
// the contacts it filed were only there to be asked about.
var errPreviewFiled = errors.New("activities: preview complete")

// previewFilingNewAddresses files the new addresses as the send would, asks the
// engine about the contacts that makes, and rolls all of it back. A preview
// that skipped the filing would answer about nobody, which the engine refuses.
func (s *Store) previewFilingNewAddresses(ctx context.Context, recipients []string, req commsauthz.Request, previewer SendPreviewer) (SendPreview, error) {
	var out SendPreview
	err := s.tx(ctx, func(tx pgx.Tx) error {
		filed, err := s.recipientFiler.FileTypedRecipientsTx(ctx, tx, recipients)
		if err != nil {
			return err
		}
		out.NewContacts = make(map[string]bool, len(filed))
		for _, address := range filed {
			out.NewContacts[address] = true
		}
		if out.Decisions, err = previewer.PreviewTx(ctx, tx, req); err != nil {
			return err
		}
		return errPreviewFiled
	})
	if errors.Is(err, errPreviewFiled) {
		err = nil
	}
	return out, err
}

// asSend is the preview as the message the send would be asked about.
func (in PreviewSendInput) asSend() SendEmailInput {
	return SendEmailInput{
		Recipients: in.Recipients, Subject: in.Subject, Body: in.Body, HTMLBody: in.HTMLBody,
		AttachmentIDs: in.AttachmentIDs, ConsentPurpose: in.LegacyPurposeKey, Context: in.Context,
		MarketingPurpose: in.MarketingPurpose, OperatorReason: in.OperatorReason, Evidence: in.Evidence,
	}
}
