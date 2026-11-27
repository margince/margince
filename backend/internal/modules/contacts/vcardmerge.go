// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergeVCardIntoTx writes a reviewed card onto the contact it resembled, in
// the caller's transaction, after a human chose "merge" over "keep separate".
//
// The deciding person must be able to read and change that contact. A refusal
// says only "not found", so a decider who cannot see the contact learns
// nothing about whether it exists. Creating a separate contact asks nothing of
// the resembled one, so the create's gate is a different one.
func (s *Store) MergeVCardIntoTx(ctx context.Context, tx pgx.Tx, target ids.ContactID, entry VCardEntry) error {
	if err := mergeTargetRefusal(auth.HoldWritableLive(ctx, tx, entityContact, target.UUID)); err != nil {
		return err
	}
	_, err := s.fillMatchedFromCard(ctx, tx, target, entry)
	return err
}

// RequireVCardMergeTarget asks the same question while the review is still
// pending, so a refused merge leaves the card waiting for another answer.
func (s *Store) RequireVCardMergeTarget(ctx context.Context, target ids.ContactID) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return mergeTargetRefusal(auth.EnsureWritableLive(ctx, tx, entityContact, target.UUID))
	})
}

// mergeTargetRefusal folds "may not change it" into "not found", as the
// import's own hidden-contact answer does.
func mergeTargetRefusal(err error) error {
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return apperrors.ErrNotFound
	}
	return err
}
