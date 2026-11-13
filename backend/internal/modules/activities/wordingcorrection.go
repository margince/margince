// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// WordingCorrectionAuthority keeps decision changes in the message owner's transaction.
type WordingCorrectionAuthority interface {
	AuthorizeWordingCorrectionTx(context.Context, pgx.Tx, ids.UUID) error
	RecordWordingCorrectionTx(context.Context, pgx.Tx, ids.UUID, commsauthz.WordingCorrection) (commsauthz.WordingCorrectionReceipt, error)
}

// CorrectHeldWording preserves the envelope and the existing decision; the
// decision's owner checks and records the human's correction in this transaction.
func (s *Store) CorrectHeldWording(ctx context.Context, intent ids.UUID, in commsauthz.WordingCorrection, authority WordingCorrectionAuthority) (int64, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return 0, err
	}
	if authority == nil {
		return 0, apperrors.ErrInvalidArgument
	}
	if err := validateWordingCorrection(in); err != nil {
		return 0, err
	}
	markup, err := SanitizeOutboundHTML(in.HTMLBody)
	if err != nil {
		return 0, err
	}
	in.HTMLBody = markup
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return 0, err
	}
	var version int64
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := authority.AuthorizeWordingCorrectionTx(ctx, tx, intent); err != nil {
			return err
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT version, status FROM scheduled_send WHERE id=@id AND scheduled_by=@seat AND held_reason=@hold_reason FOR UPDATE`,
			pgx.StrictNamedArgs{"id": intent, argSeat: actor.UserID, "hold_reason": HeldSendRefused}).Scan(&version, &status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return fmt.Errorf("scheduled send: reading the message for correction: %w", err)
		}
		if version != in.Version || status != ScheduledStatusHeld {
			return apperrors.ErrVersionSkew
		}
		receipt, err := authority.RecordWordingCorrectionTx(ctx, tx, intent, in)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `UPDATE scheduled_send
			SET payload=payload || jsonb_build_object('subject',@subject::text,'body',@body::text,'html_body',@html::text),
			version=version+1, updated_at=now() WHERE id=@id RETURNING version`, pgx.StrictNamedArgs{
			fieldSubject: in.Subject, fieldBody: in.Body, argHTML: markup, "id": intent,
		}).Scan(&version); err != nil {
			return fmt.Errorf("scheduled send: recording the wording correction: %w", err)
		}
		audit, err := storekit.AuditWithEvidence(ctx, tx, "update", "scheduled_send", intent,
			map[string]any{"wording_digest": receipt.BeforeDigest, fieldVersion: in.Version},
			map[string]any{"wording_digest": receipt.AfterDigest, fieldVersion: version},
			map[string]any{"instruction_id": receipt.InstructionID, "authorized_by": "activity.update"})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, audit, intent, crmcontracts.InternalEventScheduledWordingCorrected{Version: version})
	})
	return version, err
}

func validateWordingCorrection(in commsauthz.WordingCorrection) error {
	if err := RefuseBlankMail(in.Subject, in.Body); err != nil {
		return err
	}
	if in.Version < 1 {
		return apperrors.ErrInvalidArgument
	}
	if !in.MeaningUnchanged {
		return fmt.Errorf("confirm that only spelling or punctuation changed: %w", apperrors.ErrInvalidArgument)
	}
	if utf8.RuneCountInString(in.Subject) > maxDraftSubject {
		return &InvalidMailDraftError{Field: fieldSubject, Reason: "is too long"}
	}
	return nil
}
