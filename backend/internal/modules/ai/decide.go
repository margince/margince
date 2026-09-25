// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// TierDecideLane names the decision lane wherever a tier is reported: the
// trace, usage, the route preview. It is not a rung of any ladder — the task
// contract declares those — so no binding and no ladder ever names it.
const TierDecideLane Tier = "decide"

// LaneDecisions is the pricing lane a decision model's rate is filed under.
const LaneDecisions Lane = "decisions"

// callKindDecision is the ai_call.kind of a decision-model attempt.
const callKindDecision = "decision"

// Attempt reasons a decision attempt leaves on the ladder walk that follows
// it: why the decision did not stand.
const (
	attemptReasonDecisionBelowFloor    = "decision_below_floor"
	attemptReasonDecisionError         = "decision_error"
	attemptReasonDecisionOffEnum       = "decision_off_enum"
	attemptReasonDecisionStateTooLarge = "decision_state_too_large"
	attemptReasonDecisionUncertified   = "decision_uncertified"
	attemptReasonDecisionLocalOnly     = "decision_local_only"
)

// decisionAttemptReasons is every reason a decision attempt hands the walk
// after it, so the one test of "is this a decision reason" reads a list the
// constants above are written into once.
var decisionAttemptReasons = []string{
	attemptReasonDecisionBelowFloor, attemptReasonDecisionError, attemptReasonDecisionOffEnum,
	attemptReasonDecisionStateTooLarge, attemptReasonDecisionUncertified, attemptReasonDecisionLocalOnly,
}

func isDecisionFallbackReason(reason string) bool {
	return slices.Contains(decisionAttemptReasons, reason)
}

// DecisionCallTimeout bounds one decision call. A decision answers in well
// under two seconds (p50 about 0.6 s), and the ladder waits behind it, so a
// slow one falls back rather than stalling a background task for minutes.
const DecisionCallTimeout = 15 * time.Second

// Why a task that declares a decision form is not answered by the decision
// lane. The route preview reports one; the wire enum is the same three words.
const (
	DecisionSkipUnbound     = "unbound"
	DecisionSkipUncertified = "uncertified"
	DecisionSkipLocalOnly   = "local_only"
)

// DecisionVerdict is a site's reading of one decision answer.
type DecisionVerdict int

// The zero value is deliberately none of these, so an unset verdict is never
// read as accepted.
const (
	DecisionAccepted DecisionVerdict = iota + 1
	DecisionBelowFloor
	DecisionOffEnum
)

// DecisionGate is the site's own reading of an answer, at the floor its LLM
// path applies. The lane never judges an answer itself: the site's vocabulary
// and floors are the site's.
type DecisionGate func(decision.Answer) DecisionVerdict

// DecideOutcome is what a decision site gets back: a decision that stood, or
// the ladder's reply when it did not.
type DecideOutcome struct {
	Decided     bool
	Answer      decision.Answer // set when Decided
	Response    model.Response  // set when the ladder answered
	ServedModel string
}

// decidingLane is a lane that can ask a decision model before its ladder. Not
// every lane can, so Decide asks rather than requires.
type decidingLane interface {
	CompleteDecided(ctx context.Context, site string, dreq decision.Request, req model.Request,
		validate Validator, gate DecisionGate) (DecideOutcome, error)
}

// Decide sends a decision site's call. A lane that cannot decide — the offline
// fake, a recorder, a test double — is asked the LLM question exactly as Ask
// would ask it, so a site is written once against this seam and a lane without
// the decision form still serves it.
func Decide(ctx context.Context, lane Completer, site string, dreq decision.Request,
	req model.Request, validate Validator, gate DecisionGate,
) (DecideOutcome, error) {
	if deciding, ok := lane.(decidingLane); ok {
		return deciding.CompleteDecided(ctx, site, dreq, req, validate, gate)
	}
	resp, err := Ask(ctx, lane, req, validate)
	return DecideOutcome{Response: resp, ServedModel: resp.ServedModel}, err
}
