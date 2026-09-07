// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal's evidence entitles the product to DO about its stage.
//
// A pure function over facts the caller already read, for the reason
// AuthorSideOf is one: every row of the decision table below can then be
// written as a test, including the rows that must refuse. A policy that needed
// a database to answer would be a policy nobody could enumerate.
//
// THE FOUR OUTCOMES ARE A LADDER OF WHO DECIDES. Observe means the product
// says nothing and the deal stays where it is. Propose puts a card in front of
// a human. ProposeConfirmFirst puts the same card up but says out loud that
// this one needs a person's judgement rather than their assent. AutoApply
// moves the deal. Nothing in this file can reach AutoApply on its own — the
// caller has to have measured the transition first, which is what
// FactsAutopilot carries.

import "time"

// StageMoveOutcome is what the policy entitles the caller to do.
type StageMoveOutcome string

const (
	// OutcomeObserve records what is known and proposes nothing.
	OutcomeObserve StageMoveOutcome = "observe"
	// OutcomePropose stages a card for a human to accept or reject.
	OutcomePropose StageMoveOutcome = "propose"
	// OutcomeProposeConfirmFirst stages the same card, marked as needing a
	// judgement rather than an assent. It is what every move a mistake would
	// be expensive to undo gets, whatever the evidence says.
	OutcomeProposeConfirmFirst StageMoveOutcome = "propose_confirm_first"
	// OutcomeAutoApply moves the deal without asking. Reachable only where the
	// caller has measured this transition and the autopilot is on for it.
	OutcomeAutoApply StageMoveOutcome = "auto_apply"
)

// StageMoveDecision is the outcome and why, in words that go on the card.
type StageMoveDecision struct {
	Outcome StageMoveOutcome
	// Reason is the product's own sentence about this decision. It names the
	// fact that decided it, so a rep reading the card learns why this is being
	// asked rather than that something was computed.
	Reason string
	// Exception is set where the facts are not merely insufficient but
	// WRONG-LOOKING — a contradiction, an unhealthy source, a link nobody can
	// vouch for. Observing quietly would leave a rep with a stage that never
	// moves and no way to find out why.
	Exception string
}

// CriterionFact is one exit criterion and what the ledger says about it.
type CriterionFact struct {
	Key      string
	Required bool
	Kind     CriterionKind
	// Met is whether the standing evidence says the criterion is settled. A
	// criterion with no evidence at all is not met.
	Met bool
	// AuthorSide is who authored the evidence that settled it. Only meaningful
	// where Met; a criterion settled by no evidence has no author.
	AuthorSide AuthorSide
	// Commitment tells an agreement from a proposal. A criterion met on a
	// proposal is met on something nobody agreed to.
	Commitment string
	// Confidence is the model's, or nil where a record settled it outright. A
	// deterministic claim is not uncertain, so nil reads as certain.
	Confidence *float64
	// Contradicted marks evidence another claim disagrees with.
	Contradicted bool
}

// StageMoveFacts is everything the decision reads. Assembled in one
// transaction by the caller, so the policy sees one consistent picture.
type StageMoveFacts struct {
	Criteria []CriterionFact
	// FromTerminal and ToTerminal say whether either end of the move is a won
	// or lost stage.
	FromTerminal bool
	ToTerminal   bool
	// SingleStep is whether the target is the very next open stage. Anything
	// else is a skip or a regression.
	SingleStep bool
	// SamePipeline is false where the move would cross pipelines.
	SamePipeline bool
	// Protected marks a deal a human has recently steered: a stage move by a
	// person in the protection window, a reversal on the record, or a proposal
	// for this same target already rejected with no newer evidence since.
	Protected bool
	// ProtectedReason names which of those it was, so the Reason can say so.
	ProtectedReason string
	// SourceUnhealthy marks evidence read from a source that was not working
	// properly — a mailbox behind on sync, a failed reading.
	SourceUnhealthy bool
	// LinkageUncertain marks a deal whose activities may not all belong to it.
	LinkageUncertain bool
	// Autopilot carries the caller's measurement of this transition.
	Autopilot AutopilotFacts
}

// AutopilotFacts is what the caller has measured about letting this transition
// move itself. Every field must hold for AutoApply, and the zero value is a
// transition nobody has measured — which is the honest default.
type AutopilotFacts struct {
	// Enabled is the installation kill switch AND the per-transition mode
	// together: the caller reads both in the apply transaction and hands one
	// answer, because a policy that read them separately could be told yes by
	// a stale one.
	Enabled bool
	// Suspended marks a transition that suspended itself on its own error rate.
	Suspended bool
	// ThresholdsMet is whether the measured rates clear the bar: reviewed
	// volume, observation days, clean acceptance, correction and reversal.
	ThresholdsMet bool
}

// ambiguousConfidence is where a model's reading stops being objective enough
// to move a deal without a person looking.
//
// HIGHER than the extraction floor (0.7) on purpose. That floor answers "is
// this worth recording at all"; this one answers "is this worth acting on
// unasked", and the second question deserves the stricter answer. A claim
// between the two is recorded, shown, and confirmed by a human.
const ambiguousConfidence = 0.85

// DecideStageMove answers what this deal's evidence entitles the product to do.
//
// The order of the checks IS the policy, and it is deliberately
// refusal-first: every reason to say no is asked before any reason to say yes,
// so a deal that is both protected and fully evidenced observes. A policy that
// asked the permissive questions first would answer AutoApply for a deal a
// human had just steered by hand.
func DecideStageMove(facts StageMoveFacts) StageMoveDecision {
	if decision, refused := refuseOnTheFacts(facts); refused {
		return decision
	}
	// Everything below here has met evidence for every required criterion,
	// authored by whoever had to author it, uncontradicted, on a deal nobody
	// has recently steered.
	if reason := needsAJudgement(facts); reason != "" {
		return StageMoveDecision{Outcome: OutcomeProposeConfirmFirst, Reason: reason}
	}
	if facts.Autopilot.Enabled && !facts.Autopilot.Suspended && facts.Autopilot.ThresholdsMet {
		return StageMoveDecision{
			Outcome: OutcomeAutoApply,
			Reason:  "every exit criterion is settled by the other side's own words, and this transition has been measured long enough to move itself",
		}
	}
	return StageMoveDecision{
		Outcome: OutcomePropose,
		Reason:  "every exit criterion for this stage is settled",
	}
}

// refuseOnTheFacts asks every question whose answer is Observe.
func refuseOnTheFacts(facts StageMoveFacts) (StageMoveDecision, bool) {
	// The exception arm FIRST among the refusals, because these three are the
	// facts a rep needs told rather than merely acted on: a contradiction, a
	// source that was not working, a link nobody can vouch for. Observing them
	// silently leaves a stage that never moves and no way to find out why.
	if exception := surfacedException(facts); exception != "" {
		return StageMoveDecision{
			Outcome:   OutcomeObserve,
			Reason:    "the evidence for this stage cannot be relied on as it stands",
			Exception: exception,
		}, true
	}
	if facts.Protected {
		return StageMoveDecision{
			Outcome: OutcomeObserve,
			Reason:  facts.ProtectedReason,
		}, true
	}
	// A STAGE THAT ASKS FOR NOTHING SETTLES NOTHING. Every check below is a
	// loop over the criteria, so an empty list passes all of them vacuously —
	// and an unconfigured stage is the DEFAULT state of every stage in the
	// product, which would make this the common case rather than an edge one.
	// The reason would have read "settled by the other side's own words" about
	// a deal nobody had said anything about.
	if len(facts.Criteria) == 0 {
		return StageMoveDecision{
			Outcome: OutcomeObserve,
			Reason:  "this stage does not say what it takes to leave it, so nothing here can settle it",
		}, true
	}
	for _, c := range facts.Criteria {
		if c.Required && !c.Met {
			return StageMoveDecision{
				Outcome: OutcomeObserve,
				Reason:  "the stage still asks for " + c.Key + ", and nothing says it is settled",
			}, true
		}
		// The rule the whole ledger rests on, asked again HERE because the
		// ledger refuses such a row at the write and this reads rows already
		// written: a criterion naming something the buyer did, settled by our
		// own side's word, settles nothing. A row that predates the write-time
		// refusal, or one whose criterion kind changed after it was recorded,
		// reaches this function and must not move a deal.
		if c.Met && !SettlesBuyerMilestone(c.Kind, c.AuthorSide) {
			return StageMoveDecision{
				Outcome: OutcomeObserve,
				Reason:  c.Key + " names something the buyer does, and only our own side has said it",
			}, true
		}
	}
	return StageMoveDecision{}, false
}

// surfacedException names a fact that is wrong rather than missing.
func surfacedException(facts StageMoveFacts) string {
	for _, c := range facts.Criteria {
		if c.Contradicted {
			return "another claim disagrees with the evidence for " + c.Key
		}
	}
	if facts.SourceUnhealthy {
		return "some of this deal's evidence was read from a source that was not working properly"
	}
	if facts.LinkageUncertain {
		return "some of this deal's activity may belong to another record"
	}
	return ""
}

// needsAJudgement names why a move a human should look at is one, or "" where
// the move is ordinary.
//
// Each of these is a move whose MISTAKE is expensive, independently of how
// good the evidence is. Winning a deal that is not won, skipping a stage the
// pipeline exists to enforce, or acting on a date somebody floated are all
// errors a rep would rather catch on a card than find later in a forecast.
func needsAJudgement(facts StageMoveFacts) string {
	if facts.FromTerminal || facts.ToTerminal {
		return "this move enters or leaves a closing stage, which is always a person's call"
	}
	if !facts.SamePipeline {
		return "this move would cross into another pipeline"
	}
	if !facts.SingleStep {
		return "this move skips or goes back a stage rather than advancing by one"
	}
	for _, c := range facts.Criteria {
		if !c.Met {
			// An unmet OPTIONAL criterion does not block the move — the stage
			// said it was optional — but it is not nothing either: a stage
			// carrying an unsettled criterion of any kind is one a person
			// should glance at before the deal moves itself.
			return c.Key + " is not settled, and the stage lists it"
		}
		// Anything but an explicit agreement. CommitmentNone is a criterion
		// settled by a record rather than by anybody's undertaking — a
		// signature, a meeting that happened — and that is objective. An EMPTY
		// commitment is a row that never said, and a policy treating silence
		// as agreement would act on the one value nobody chose.
		if c.Commitment != CommitmentAgreed && c.Commitment != CommitmentNone {
			return c.Key + " does not rest on anything the other side agreed to"
		}
		if c.Confidence != nil && *c.Confidence < ambiguousConfidence {
			return "the reading of " + c.Key + " is not certain enough to act on unasked"
		}
	}
	return ""
}

// ProtectionWindow is how long a human's own stage move keeps the product from
// proposing another.
//
// A fortnight, because a rep who moved a deal by hand has said where they
// think it is, and a proposal contradicting them a day later is the product
// arguing with the person it works for. Long enough to cover a sales cycle's
// natural pause; short enough that a deal does not go unread for a quarter.
const ProtectionWindow = 14 * 24 * time.Hour
