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
	// later holds the checks both paths share, and the path to a later moment.
	later sendlater.Service
}

func (s directedSendService) DirectAndSend(
	ctx context.Context, reviewID ids.UUID, in consent.DirectInput,
) (crmcontracts.Activity, error) {
	admitted, err := s.later.Admit(ctx, reviewID, in)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	// A new grant can resolve the old refusal without a human exception.
	// ResumeHeldSend rechecks and rolls back a send that remains refused.
	sent, err := s.sends.ResumeHeldSend(ctx, admitted.Intent, s.gate, s.delivery)
	if !errors.Is(err, apperrors.ErrConsentNotGranted) {
		return sent, s.consent.SentInstead(ctx, reviewID, err)
	}
	if pending, ok := errors.AsType[*pendingReviewError](err); ok {
		if err := s.later.RefuseIfChanged(ctx, admitted, pending.set, pending.cause); err != nil {
			return crmcontracts.Activity{}, err
		}
	}
	if _, err := s.consent.DirectSend(ctx, reviewID, admitted.In); err != nil &&
		!errors.Is(err, consent.ErrDecisionAlreadyRecorded) {
		return crmcontracts.Activity{}, s.consent.SentInstead(ctx, reviewID, err)
	}
	sent, err = s.sends.ResumeHeldSend(ctx, admitted.Intent, s.gate, s.delivery)
	if err != nil {
		return crmcontracts.Activity{}, s.consent.SentInstead(ctx, reviewID, fmt.Errorf(
			"the decision was recorded and the message could not be sent on it: %w", err))
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
	db := InstallationDB(pool)
	consents, sends, gate := consent.NewStore(db), sendStore(pool, send), consentGateFor(pool)
	return directedSendService{
		consent: consents, sends: sends, gate: gate, delivery: delivery,
		later: sendlater.Service{
			Consent: consents, Sends: sends, Gate: gate, Delivery: delivery,
			Timer: send.ScheduleTimer, DB: db, Actions: actionsForRefusal,
		},
	}
}
