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
)

func (p capturedTextPurger) FlagSubjectReviewTx(ctx context.Context, tx pgx.Tx, subject ids.ContactID, sources []privacy.ErasureReviewSource) error {
	system := principal.SystemActing(ctx, "system:privacy_review")
	if correlation, ok := principal.CorrelationID(ctx); ok {
		system = principal.WithCorrelationID(system, correlation)
	}
	store := notices.NewStore(p.db)
	for _, source := range sources {
		target := notices.Target{Type: source.Kind, ID: source.ID}
		var recipients []ids.UserID
		if source.Kind == "email_signature" && source.Owner != nil {
			live, err := identity.LiveReviewRecipientTx(system, tx, *source.Owner)
			if err != nil {
				return err
			}
			if live {
				recipients = []ids.UserID{ids.From[ids.UserKind](*source.Owner)}
			} else {
				target = notices.Target{}
			}
		}
		if len(recipients) == 0 {
			object := source.Kind
			if object == "signature_template" {
				object = "installation_settings"
			}
			if object == "email_signature" {
				object = "contact"
			}
			var err error
			recipients, err = identity.PrivacyReviewOperatorsTx(system, tx, object)
			if err != nil {
				return err
			}
		}
		if len(recipients) == 0 {
			return &PrivacyReviewUnavailableError{}
		}
		body := manualPrivacyReviewText(source.Kind)
		if source.Kind == "email_signature" && target.Type == "" && source.Owner != nil {
			body = fmt.Sprintf("An inactive member's retained signature needs manual privacy review. Signature reference: %s. Member reference: %s. An administrator must review it before that member is reactivated.", source.ID, *source.Owner)
		}
		for _, recipient := range recipients {
			_, err := store.CreateTx(system, tx, notices.NewNotice{Recipient: recipient, Kind: notices.KindPrivacyReviewRequired,
				Subject: "Manual privacy review needed", Body: body, Target: target,
				DedupeKey: fmt.Sprintf("privacy-review:%s:%s:%s", subject, source.Kind, source.ID),
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func manualPrivacyReviewText(kind string) string {
	switch kind {
	case "knowledge_document":
		return "An uploaded document may mention an erased contact. Keep the file and open its original to review the text."
	case "email_signature":
		return "An email signature may mention an erased contact. Review the signature before deciding what to remove."
	case "activity":
		return "This retained note may mention an erased contact. Review the possible match manually; its text was kept."
	case "legal_entity", "signature_template":
		return "A company email signature template may mention an erased contact. Review it in Company settings before deciding what to remove."
	default:
		return "This record's description may mention an erased contact. Review its text before deciding what to remove."
	}
}

type PrivacyReviewUnavailableError struct{}

func (*PrivacyReviewUnavailableError) Error() string {
	return "No active privacy reviewer can read the matching retained content. An administrator must grant a human administrator or operations member unrestricted record access and contact deletion permission, then retry erasure."
}
func (*PrivacyReviewUnavailableError) Unwrap() error                    { return apperrors.ErrConflict }
func (e *PrivacyReviewUnavailableError) MessageFault() (string, string) { return "conflict", e.Error() }
