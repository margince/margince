// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// admitDirector is DirectSend's checks on the caller and what they typed: a
// human at the keyboard, the exception grant and a valid acknowledgement.
func admitDirector(ctx context.Context, in DirectInput) (DirectInput, error) {
	if err := requireAHumanAtTheKeyboard(ctx); err != nil {
		return DirectInput{}, err
	}
	if err := auth.Require(ctx, entityCommunicationException, principal.ActionCreate); err != nil {
		return DirectInput{}, err
	}
	return in.Validated()
}

// CheckDirection runs every check DirectSend runs, and writes nothing. The
// direct-send door asks it before it tries the held message again, so a caller
// who could not direct it cannot release it either.
func (s *Store) CheckDirection(ctx context.Context, reviewID ids.UUID, in DirectInput) (DirectInput, error) {
	in, err := admitDirector(ctx, in)
	if err != nil {
		return DirectInput{}, err
	}
	return in, s.db.Tx(ctx, func(tx pgx.Tx) error {
		review, err := claimReviewForDirectionTx(ctx, tx, reviewID)
		if err != nil {
			return err
		}
		return review.admits(ctx, in)
	})
}

// admits checks that this caller may direct the review as they read it. It
// asks for the revision they confirmed, the refusals they were shown and a
// warning the review was served with.
func (r directableReview) admits(ctx context.Context, in DirectInput) error {
	if err := (Review{Refusals: r.Refusals}).checkAcknowledgement(in.AssessmentRevision); err != nil {
		return err
	}
	if err := mayDirect(ctx, in.ShownReasons, r.Refusals); err != nil {
		return err
	}
	if !servesWarning(warningsServed(ctx, r.InitiatedBy, r.IntentID), in.WarningVersion) {
		return &ValidationError{
			Field:  fieldWarningVersion,
			Reason: "this review was not shown with that warning; open it again and confirm the warning it shows",
		}
	}
	return nil
}

// WrongWarningVersion reports whether a direction was refused because it named
// a warning this build does not serve, or one this review was not shown with.
func WrongWarningVersion(err error) bool {
	refusal, ok := errors.AsType[*ValidationError](err)
	return ok && refusal.Field == fieldWarningVersion
}
