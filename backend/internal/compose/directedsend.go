// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Directing a refused message out, end to end.
//
// Two things happen and they are separate on purpose. consent records the
// DECISION: a named human read what the engine refused, was shown a warning,
// acknowledged it, and said in writing why this message goes. activities fires
// the held MESSAGE that decision was about, through the ordinary send.
//
// The join between them is the held scheduled_send. An instruction is given
// against a review; the review names the held row; resuming that row carries
// its id to the staging path, which finds the decision and spends it inside the
// transaction that stages the delivery.
//
// WHY COMPOSE OWNS THIS. consent may not reach into activities and activities
// may not reach into consent — a module never imports a sibling. The decision
// and the send are two modules' work, and the order between them is this
// layer's to hold.
//
// THE ORDER IS DECISION FIRST. If the send were attempted first there would be
// a moment where a message had gone out with no recorded authority behind it,
// which is the one state this whole design exists to prevent. Recording first
// risks the opposite — a decision with no message — and that is recoverable:
// the instruction stands unspent, the held message is still held, and the rep
// can try again.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// directedSendService carries out a decision to send a refused message.
type directedSendService struct {
	consent  *consent.Store
	sends    *activities.Store
	gate     *consent.Gate
	delivery DeliveryMachinery
}

// DirectAndSend records the decision and fires the message it was about.
//
// The review is read first for the id of the message it holds, because a
// decision against a review with no held message can be recorded and never
// carried out — and telling the rep that before anything is written is better
// than leaving them an instruction they cannot spend.
func (s directedSendService) DirectAndSend(
	ctx context.Context, reviewID ids.UUID, in consent.DirectInput,
) (crmcontracts.Activity, error) {
	// READ UNSCOPED, gated on the same grant directing needs. The initiator's
	// own read would refuse a designated reviewer acting on somebody else's
	// refusal, which is exactly the human this path exists for.
	intent, err := s.consent.HeldMessageForReview(ctx, reviewID)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if intent.IsZero() {
		return crmcontracts.Activity{}, &consent.InstructionStaleError{
			Why: "this review holds no message to send — it was refused before the message could be kept",
		}
	}
	// RECORDING TWICE IS NOT AN ERROR TO REPORT, it is the retry working.
	//
	// The decision commits before the send is attempted, and a send can fail
	// for reasons that have nothing to do with the decision — a mailbox that
	// cannot be resolved, a provider seam that is not wired. The reviewer's
	// obvious next move is to press the button again, and the review carries a
	// unique key, so the second attempt would be refused for a duplicate
	// decision and the standing one could never be spent. They would be
	// trapped by their own retry.
	//
	// A standing decision on this review is THIS decision: it names the same
	// review, it was made by a human who acknowledged the same warning, and it
	// has not been spent. So the retry goes on to the send rather than trying
	// to record it again.
	if _, err := s.consent.DirectSend(ctx, reviewID, in); err != nil &&
		!errors.Is(err, consent.ErrDecisionAlreadyRecorded) {
		return crmcontracts.Activity{}, err
	}
	sent, err := s.sends.ResumeHeldSend(ctx, intent, s.gate, s.delivery)
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
