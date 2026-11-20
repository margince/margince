// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/sendlater"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

type directedSendService struct {
	later    sendlater.Service
	gate     *consent.Gate
	delivery DeliveryMachinery
}

func (s directedSendService) DirectAndSend(
	ctx context.Context, reviewID ids.UUID, in consent.DirectInput,
) (crmcontracts.Activity, error) {
	intent, err := s.later.Consent.HeldMessageForReview(ctx, reviewID)
	if err != nil {
		return crmcontracts.Activity{}, s.later.Consent.SentInstead(ctx, reviewID, err)
	}
	if intent.IsZero() {
		return crmcontracts.Activity{}, consent.ErrNothingHeld
	}
	previous, err := s.later.Consent.ReviewForReader(ctx, reviewID)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	// Every check DirectSend runs comes first, the author check included, so a
	// colleague is refused before anything is tried, recorded or sent. Only a
	// caller who may direct the message may release it, even when a new grant
	// would now allow it.
	if in, err = s.later.Consent.CheckDirection(ctx, reviewID, in); err != nil {
		return crmcontracts.Activity{}, s.later.Consent.SentInstead(ctx, reviewID, err)
	}
	// A new grant can resolve the old refusal without a human exception.
	// ResumeHeldSend rechecks and rolls back a send that remains refused.
	sent, err := s.later.Sends.ResumeHeldSend(ctx, intent, s.gate, s.delivery)
	if !errors.Is(err, apperrors.ErrConsentNotGranted) {
		return sent, s.later.Consent.SentInstead(ctx, reviewID, err)
	}
	var pending *pendingReviewError
	if errors.As(err, &pending) && pending.set.ExceptionMayAnswer(false) && !previous.MatchesAssessment(pending.set) {
		refreshed, refreshErr := s.gate.RefreshRefusal(ctx, pending.set, intent)
		if refreshErr != nil {
			return crmcontracts.Activity{}, refreshErr
		}
		return crmcontracts.Activity{}, s.gate.Refused(ctx, refreshed, pending.set, pending.cause, actionsForRefusal(ctx, refreshed))
	}
	if _, err := s.later.Consent.DirectSend(ctx, reviewID, in); err != nil &&
		!errors.Is(err, consent.ErrDecisionAlreadyRecorded) {
		return crmcontracts.Activity{}, s.later.Consent.SentInstead(ctx, reviewID, err)
	}
	sent, err = s.later.Sends.ResumeHeldSend(ctx, intent, s.gate, s.delivery)
	if err != nil {
		return crmcontracts.Activity{}, s.later.Consent.SentInstead(ctx, reviewID, fmt.Errorf(
			"the decision was recorded and the message could not be sent on it: %w", err))
	}
	return sent, nil
}

func (s directedSendService) CorrectWording(ctx context.Context, reviewID ids.UUID, in commsauthz.WordingCorrection) (int64, error) {
	intent, err := s.later.Consent.HeldMessageForReview(ctx, reviewID)
	if err != nil {
		return 0, err
	}
	if intent.IsZero() {
		return 0, apperrors.ErrNotFound
	}
	return s.later.Sends.CorrectHeldWording(ctx, intent, in, consent.WordingCorrectorForReview(reviewID))
}

func newDirectedSendService(pool *pgxpool.Pool, delivery DeliveryMachinery, send SendPath) directedSendService {
	return directedSendService{
		later:    sendlater.Service{Consent: consent.NewStore(InstallationDB(pool)), Sends: sendStore(pool, send), Timer: send.ScheduleTimer},
		gate:     consentGateFor(pool),
		delivery: delivery,
	}
}
