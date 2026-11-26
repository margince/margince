// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// PrivacyReviewer admits the privacy reviewer inside the caller's transaction.
// The contacts module cannot read who holds that duty, so startup wiring
// injects the identity check.
type PrivacyReviewer interface {
	RequirePrivacyReviewTx(context.Context, pgx.Tx, string) error
}

// WithPrivacyReviewer sets the check that guards retained signatures. Without
// one, every read and save of a retained signature is refused.
func (s *Store) WithPrivacyReviewer(reviewer PrivacyReviewer) *Store {
	s.privacyReviewer = reviewer
	return s
}

// RetainedSignature is the email signature a departed member left behind,
// with the version a save must name and the sanitized preview.
type RetainedSignature struct {
	ID, OwnerID ids.UUID
	Version     int64
	OwnerName   string
	Signature   EmailSignature
	PreviewHTML string
}

// GetRetainedSignature reads a departed member's signature for the privacy
// reviewer. A live member's signature answers not found.
func (s *Store) GetRetainedSignature(ctx context.Context, id ids.UUID) (RetainedSignature, error) {
	var out RetainedSignature
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.requireSignatureReview(ctx, tx); err != nil {
			return err
		}
		var err error
		out, err = s.readRetainedSignatureTx(ctx, tx, id, false)
		return err
	})
	return out, err
}

// SaveRetainedSignature replaces a departed member's signature when expected
// matches the stored version, so the reviewer saves only the text they read.
func (s *Store) SaveRetainedSignature(ctx context.Context, id ids.UUID, expected int64, in SaveSignatureInput) (RetainedSignature, error) {
	in, err := validatedSignature(in)
	if err != nil {
		return RetainedSignature{}, err
	}
	var out RetainedSignature
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.requireSignatureReview(ctx, tx); err != nil {
			return err
		}
		before, err := s.readRetainedSignatureTx(ctx, tx, id, false)
		if err != nil {
			return err
		}
		// Invitation redemption takes the member row, without the authorization lock.
		// Hold it before the signature lock so eligibility cannot change underneath it.
		if err := tx.QueryRow(ctx, `SELECT id FROM app_user WHERE id=$1 FOR SHARE`, before.OwnerID).Scan(&before.OwnerID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return err
		}
		if err := storekit.LockWriteIdentity(ctx, tx, "email_signature", before.OwnerID.String()); err != nil {
			return err
		}
		before, err = s.readRetainedSignatureTx(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if before.Version != expected {
			return &apperrors.VersionSkewError{Message: "The retained signature changed. Reload it and review the current text before saving."}
		}
		if err := saveSignatureTx(ctx, tx, before.OwnerID, before.Signature, in, false); err != nil {
			return err
		}
		out, err = s.readRetainedSignatureTx(ctx, tx, id, false)
		return err
	})
	return out, err
}

func (s *Store) requireSignatureReview(ctx context.Context, tx pgx.Tx) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || s.privacyReviewer == nil {
		return apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "contact", principal.ActionDelete); err != nil {
		return err
	}
	return s.privacyReviewer.RequirePrivacyReviewTx(ctx, tx, "contact")
}

func (s *Store) readRetainedSignatureTx(ctx context.Context, tx pgx.Tx, id ids.UUID, lock bool) (RetainedSignature, error) {
	var out RetainedSignature
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF s FOR SHARE OF u"
	}
	err := tx.QueryRow(ctx, `SELECT s.id, s.owner_id, u.display_name, s.version,
 s.body, s.html, s.title, s.phone, s.updated_at
 FROM email_signature s JOIN app_user u ON u.id=s.owner_id
 WHERE s.id=@id AND NOT (`+workforce.LiveHumanSQL("u")+`)`+suffix, pgx.StrictNamedArgs{"id": id}).
		Scan(&out.ID, &out.OwnerID, &out.OwnerName, &out.Version, &out.Signature.Body, &out.Signature.HTML,
			&out.Signature.Title, &out.Signature.Phone, &out.Signature.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RetainedSignature{}, apperrors.ErrNotFound
	}
	if err != nil {
		return RetainedSignature{}, err
	}
	if s.signaturePreview != nil {
		out.PreviewHTML, err = s.signaturePreview(out.Signature.HTML)
	}
	return out, err
}

// WithRetainedSignaturePreview sets the renderer that fills PreviewHTML. It is
// injected so the preview uses the same sanitizer as the send path.
func (s *Store) WithRetainedSignaturePreview(preview func(string) (string, error)) *Store {
	s.signaturePreview = preview
	return s
}
