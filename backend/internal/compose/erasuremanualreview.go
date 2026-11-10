// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

const (
	reviewEmailSignature    = "email_signature"
	reviewSignatureTemplate = "signature_template"
	reviewLegalEntity       = "legal_entity"
	reviewSettingsObject    = "installation_settings"
)

func (p capturedTextPurger) FlagSubjectReviewTx(ctx context.Context, tx pgx.Tx, subject ids.ContactID, sources []privacy.ErasureReviewSource) error {
	system := principal.SystemActing(ctx, "system:privacy_review")
	if correlation, ok := principal.CorrelationID(ctx); ok {
		system = principal.WithCorrelationID(system, correlation)
	}
	store := notices.NewStore(p.db)
	for _, source := range sources {
		target, recipients, err := privacyReviewRecipients(system, tx, source)
		if err != nil {
			return err
		}
		body := manualPrivacyReviewText(source.Kind)
		metadata := &systemtext.Message{Subject: systemtext.Sentence("privacy.review.subject"), Body: systemtext.Sentence(manualPrivacyReviewCode(source.Kind))}
		if source.Kind == reviewEmailSignature && target.Type == notices.TargetRetainedEmailSignature && source.Owner != nil {
			metadata.Body = systemtext.Sentence("privacy.review.retained_signature")
			body = systemtext.Literal("privacy.review.retained_signature")
		}
		for _, recipient := range recipients {
			_, err := store.CreateTx(system, tx, notices.NewNotice{
				Recipient: recipient, Kind: notices.KindPrivacyReviewRequired,
				SystemText: metadata,
				Subject:    systemtext.Literal("privacy.review.subject"), Body: body, Target: target,
				DedupeKey: fmt.Sprintf("privacy-review:%s:%s:%s", subject, source.Kind, source.ID),
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func privacyReviewRecipients(ctx context.Context, tx pgx.Tx, source privacy.ErasureReviewSource) (notices.Target, []ids.UserID, error) {
	target := notices.Target{Type: source.Kind, ID: source.ID}
	if source.Kind == reviewEmailSignature && source.Owner != nil {
		live, err := identity.LiveReviewRecipientTx(ctx, tx, *source.Owner)
		if err != nil {
			return target, nil, err
		}
		if live {
			return target, []ids.UserID{ids.From[ids.UserKind](*source.Owner)}, nil
		}
		target = notices.Target{Type: notices.TargetRetainedEmailSignature, ID: source.ID}
	}
	object := source.Kind
	switch object {
	case reviewSignatureTemplate:
		object = reviewSettingsObject
	case reviewEmailSignature:
		object = string(recordTypeContact)
	}
	recipients, err := identity.PrivacyReviewOperatorsTx(ctx, tx, object)
	if err != nil {
		return target, nil, err
	}
	if len(recipients) == 0 {
		return target, nil, &privacyReviewUnavailableError{}
	}
	return target, recipients, nil
}

func manualPrivacyReviewText(kind string) string {
	return systemtext.Literal(manualPrivacyReviewCode(kind))
}

func manualPrivacyReviewCode(kind string) string {
	switch kind {
	case "knowledge_document":
		return "privacy.review.document"
	case reviewEmailSignature:
		return "privacy.review.signature"
	case string(recordTypeActivity):
		return "privacy.review.note"
	case reviewLegalEntity, reviewSignatureTemplate:
		return "privacy.review.company_signature"
	default:
		return "privacy.review.description"
	}
}

type privacyReviewUnavailableError struct{}

func (*privacyReviewUnavailableError) Error() string {
	return "No active privacy reviewer can read the matching retained content. An administrator must grant a human administrator or operations member unrestricted record access and contact deletion permission, then retry erasure."
}
func (*privacyReviewUnavailableError) Unwrap() error { return apperrors.ErrConflict }

// MessageFault tells the caller how an administrator can restore required review access.
func (e *privacyReviewUnavailableError) MessageFault() (string, string) { return "conflict", e.Error() }
