// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package privacyreview raises manual privacy reviews for retained content that
// an erasure could not safely remove, and the audit correction steps that need them.
package privacyreview

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
	reviewContact           = "contact"
	reviewActivity          = "activity"
)

// UnprovenSubjectScope keys reviews that no erased subject can be named for.
// Each source ID then identifies its review on its own.
const UnprovenSubjectScope = "unproven"

// FlagTx tells the reviewers of each source. It returns how many sources got a
// new notice; the scope and source make the dedupe key, so a replay counts none.
func FlagTx(ctx context.Context, tx pgx.Tx, store *notices.Store, scope string, sources []privacy.ErasureReviewSource) (int, error) {
	system := principal.SystemActing(ctx, "system:privacy_review")
	if correlation, ok := principal.CorrelationID(ctx); ok {
		system = principal.WithCorrelationID(system, correlation)
	}
	flagged := 0
	for _, source := range sources {
		told := false
		target, recipients, err := reviewersFor(system, tx, source)
		if err != nil {
			return flagged, err
		}
		body := manualPrivacyReviewText(source.Kind)
		metadata := &systemtext.Message{Subject: systemtext.Sentence("privacy.review.subject"), Body: systemtext.Sentence(manualPrivacyReviewCode(source.Kind))}
		if source.Kind == reviewEmailSignature && target.Type == notices.TargetRetainedEmailSignature && source.Owner != nil {
			metadata.Body = systemtext.Sentence("privacy.review.retained_signature")
			body = systemtext.Literal("privacy.review.retained_signature")
		}
		for _, recipient := range recipients {
			_, fresh, err := store.CreateTxNew(system, tx, notices.NewNotice{
				Recipient: recipient, Kind: notices.KindPrivacyReviewRequired,
				SystemText: metadata,
				Subject:    systemtext.Literal("privacy.review.subject"), Body: body, Target: target,
				DedupeKey: fmt.Sprintf("privacy-review:%s:%s:%s", scope, source.Kind, source.ID),
			})
			if err != nil {
				return flagged, err
			}
			told = told || fresh
		}
		if told {
			flagged++
		}
	}
	return flagged, nil
}

func reviewersFor(ctx context.Context, tx pgx.Tx, source privacy.ErasureReviewSource) (notices.Target, []ids.UserID, error) {
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
		object = reviewContact
	case privacy.ApprovalHistoryReview:
		object = "audit_log"
	}
	recipients, err := identity.PrivacyReviewOperatorsTx(ctx, tx, object)
	if err != nil {
		return target, nil, err
	}
	if len(recipients) == 0 {
		return target, nil, &UnavailableError{}
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
	case reviewActivity:
		return "privacy.review.note"
	case reviewLegalEntity, reviewSignatureTemplate:
		return "privacy.review.company_signature"
	case privacy.ApprovalHistoryReview:
		return "privacy.review.approval_history"
	default:
		return "privacy.review.description"
	}
}

// UnavailableError refuses a review that no live reviewer could read.
type UnavailableError struct{}

func (*UnavailableError) Error() string {
	return "No active privacy reviewer can read the matching retained content. An administrator must grant a human administrator or operations member unrestricted record access and contact deletion permission, then retry erasure."
}
func (*UnavailableError) Unwrap() error { return apperrors.ErrConflict }

// MessageFault tells the caller how an administrator can restore required review access.
func (e *UnavailableError) MessageFault() (string, string) { return "conflict", e.Error() }
