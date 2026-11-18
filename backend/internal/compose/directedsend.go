// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

type directedSendService struct {
	consent  *consent.Store
	sends    *activities.Store
	gate     *consent.Gate
	delivery DeliveryMachinery
}

func (s directedSendService) DirectAndSend(
	ctx context.Context, reviewID ids.UUID, in consent.DirectInput,
) (crmcontracts.Activity, error) {
	intent, err := s.consent.HeldMessageForReview(ctx, reviewID)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if intent.IsZero() {
		return crmcontracts.Activity{}, &consent.InstructionStaleError{
			Why: "this review holds no message to send — it was refused before the message could be kept",
		}
	}
	previous, err := s.consent.ReviewForReader(ctx, reviewID)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	// Every check DirectSend runs comes first. Only a caller who may direct the
	// message may release it, even when a new grant would now allow it.
	if in, err = s.consent.CheckDirection(ctx, reviewID, in); err != nil {
		return crmcontracts.Activity{}, err
	}
	// A new grant can resolve the old refusal without a human exception.
	// ResumeHeldSend rechecks and rolls back a send that remains refused.
	sent, err := s.sends.ResumeHeldSend(ctx, intent, s.gate, s.delivery)
	if !errors.Is(err, apperrors.ErrConsentNotGranted) {
		return sent, err
	}
	var pending *pendingReviewError
	if errors.As(err, &pending) && pending.set.ExceptionMayAnswer(false) && !previous.MatchesAssessment(pending.set) {
		refreshed, refreshErr := s.gate.RefreshRefusal(ctx, pending.set, intent)
		if refreshErr != nil {
			return crmcontracts.Activity{}, refreshErr
		}
		return crmcontracts.Activity{}, s.gate.Refused(ctx, refreshed, pending.set, pending.cause, actionsForRefusal(ctx, refreshed))
	}
	if _, err := s.consent.DirectSend(ctx, reviewID, in); err != nil &&
		!errors.Is(err, consent.ErrDecisionAlreadyRecorded) {
		return crmcontracts.Activity{}, err
	}
	sent, err = s.sends.ResumeHeldSend(ctx, intent, s.gate, s.delivery)
	if err != nil {
		return crmcontracts.Activity{}, fmt.Errorf(
			"the decision was recorded and the message could not be sent on it: %w", err)
	}
	return sent, nil
}

func (s directedSendService) CorrectWording(ctx context.Context, reviewID ids.UUID, in commsauthz.WordingCorrection) (int64, error) {
	intent, err := s.consent.HeldMessageForReview(ctx, reviewID)
	if err != nil {
		return 0, err
	}
	if intent.IsZero() {
		return 0, apperrors.ErrNotFound
	}
	return s.sends.CorrectHeldWording(ctx, intent, in, consent.WordingCorrectorForReview(reviewID))
}

func newDirectedSendService(pool *pgxpool.Pool, delivery DeliveryMachinery, send SendPath) directedSendService {
	return directedSendService{
		consent:  consent.NewStore(InstallationDB(pool)),
		sends:    sendStore(pool, send),
		gate:     consentGateFor(pool),
		delivery: delivery,
	}
}
