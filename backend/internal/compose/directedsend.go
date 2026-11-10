// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Consent records a human exception; activities resumes the held message under
// current checks. Compose joins them without a module importing its sibling.

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
)

// directedSendService carries out a decision to send a refused message.
type directedSendService struct {
	consent  *consent.Store
	sends    *activities.Store
	gate     *consent.Gate
	delivery DeliveryMachinery
}

// DirectAndSend resumes a permitted message or records an eligible exception first.
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
	in, err = in.Validated()
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	// A new grant can resolve the old refusal without a human exception.
	// ResumeHeldSend rechecks and rolls back a send that remains refused.
	sent, err := s.sends.ResumeHeldSend(ctx, intent, s.gate, s.delivery)
	if !errors.Is(err, apperrors.ErrConsentNotGranted) {
		return sent, err
	}
	// A retry may already have recorded the same unspent instruction.
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

// newDirectedSendService wires the two modules and the machinery between them.
func newDirectedSendService(pool *pgxpool.Pool, delivery DeliveryMachinery, send SendPath) directedSendService {
	return directedSendService{
		consent:  consent.NewStore(InstallationDB(pool)),
		sends:    sendStore(pool, send),
		gate:     consentGateFor(pool),
		delivery: delivery,
	}
}
