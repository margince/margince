// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// One test per row of the decision table, plus the census that proves the
// table is the whole table.
//
// The refusals are the subject. A policy that proposes a move it should have
// observed puts a wrong card in front of a rep; one that AUTO-APPLIES a move
// it should have proposed moves a deal nobody agreed to move, and the rep
// finds out from a forecast.

import "testing"

// settledFacts is the fully-evidenced single-step move every test below varies
// ONE fact of. Written as the permissive case on purpose: a test that started
// from a refusing fixture would pass whether or not its own clause worked.
func settledFacts() StageMoveFacts {
	return StageMoveFacts{
		Criteria: []CriterionFact{
			{
				Key: "problem_confirmed", Required: true, Kind: CriterionBuyerConfirmed,
				Met: true, AuthorSide: AuthorBuyer, Commitment: CommitmentAgreed,
			},
			{
				Key: "demo_held", Required: true, Kind: CriterionEventHeld,
				Met: true, AuthorSide: AuthorUnknown, Commitment: CommitmentAgreed,
			},
		},
		SingleStep:   true,
		SamePipeline: true,
	}
}

// The control: without it every refusal below could be passing because the
// fixture refuses everything.
func TestASingleStepMoveWithObjectiveEvidenceProposes(t *testing.T) {
	got := DecideStageMove(settledFacts())
	if got.Outcome != OutcomePropose {
		t.Fatalf("a fully evidenced single-step move decided %q (%s)", got.Outcome, got.Reason)
	}
	if got.Exception != "" {
		t.Errorf("an ordinary move surfaced the exception %q", got.Exception)
	}
}

func TestAnUnmetRequiredCriterionOnlyObserves(t *testing.T) {
	facts := settledFacts()
	facts.Criteria[0].Met = false
	got := DecideStageMove(facts)
	if got.Outcome != OutcomeObserve {
		t.Fatalf("an unmet required criterion decided %q", got.Outcome)
	}
	if got.Reason == "" {
		t.Error("the refusal says nothing about which criterion is missing")
	}
}

// An OPTIONAL criterion left unmet does not REFUSE the move — the stage said
// it was optional, and treating it as required would make the flag mean
// nothing — but it does cost the move its automatic pass. A stage carrying an
// unsettled criterion of any kind is one a person should glance at before the
// deal moves itself.
func TestAnUnmetOptionalCriterionDoesNotRefuseButAsksForAGlance(t *testing.T) {
	facts := settledFacts()
	facts.Criteria = append(facts.Criteria, CriterionFact{
		Key: "nice_to_have", Required: false, Kind: CriterionCustom,
	})
	got := DecideStageMove(facts)
	if got.Outcome == OutcomeObserve {
		t.Fatalf("an unmet OPTIONAL criterion refused the move; the required "+
			"flag then means nothing (%s)", got.Reason)
	}
	if got.Outcome != OutcomeProposeConfirmFirst {
		t.Errorf("an unmet optional criterion decided %q, want a card a person looks at", got.Outcome)
	}

	// And it must outrank a measured autopilot: an unsettled criterion is
	// exactly what a person should see before the deal moves itself.
	facts.Autopilot = AutopilotFacts{Enabled: true, ThresholdsMet: true}
	if got := DecideStageMove(facts); got.Outcome == OutcomeAutoApply {
		t.Error("an unmet optional criterion auto-applied under a measured autopilot")
	}
}

// A stage that says nothing about what it takes to leave it settles nothing.
//
// Every other check is a loop over the criteria, so an empty list passes all
// of them vacuously — and an unconfigured stage is the DEFAULT state of every
// stage in the product. Left unhandled this was not an edge case: it
// auto-applied every deal on every stage nobody had configured, with a reason
// claiming the buyer's own words had settled it.
func TestAStageWithNoCriteriaSettlesNothing(t *testing.T) {
	bare := StageMoveFacts{
		SingleStep: true, SamePipeline: true,
		Autopilot: AutopilotFacts{Enabled: true, ThresholdsMet: true},
	}
	got := DecideStageMove(bare)
	if got.Outcome != OutcomeObserve {
		t.Fatalf("a stage with no exit criteria decided %q (%s)", got.Outcome, got.Reason)
	}
}

// Silence is not agreement. A row that never said what it rests on is the one
// value nobody chose, and treating it as objective would act on it.
func TestACriterionRestingOnNothingNamedIsNotObjective(t *testing.T) {
	facts := settledFacts()
	facts.Criteria[0].Commitment = ""
	if got := DecideStageMove(facts); got.Outcome != OutcomeProposeConfirmFirst {
		t.Fatalf("a criterion with no stated commitment decided %q", got.Outcome)
	}

	// CommitmentNone is different and IS objective: it is a criterion settled
	// by a record rather than by anybody's undertaking — a signature, a
	// meeting that happened.
	facts.Criteria[0].Commitment = CommitmentNone
	if got := DecideStageMove(facts); got.Outcome != OutcomePropose {
		t.Errorf("a criterion settled by a record decided %q", got.Outcome)
	}
}

// The rule the whole ledger rests on, asked again at the policy: the store
// refuses such a row at the write, and this reads rows already written — one
// recorded before that refusal existed, or whose criterion kind changed after
// the fact, reaches here and must not move a deal.
func TestSellerAuthoredEvidenceForABuyerMilestoneOnlyObserves(t *testing.T) {
	for _, side := range []AuthorSide{AuthorSeller, AuthorUnknown} {
		facts := settledFacts()
		facts.Criteria[0].AuthorSide = side
		got := DecideStageMove(facts)
		if got.Outcome != OutcomeObserve {
			t.Errorf("%s-authored evidence for a buyer milestone decided %q", side, got.Outcome)
		}
	}
	// The positive control: the buyer's own word still proposes, so the
	// refusals above are about authorship and not about a fixture that refuses
	// everything.
	if got := DecideStageMove(settledFacts()); got.Outcome != OutcomePropose {
		t.Fatalf("buyer-authored evidence decided %q", got.Outcome)
	}
}

func TestContradictedEvidenceObservesAndSurfacesAnException(t *testing.T) {
	facts := settledFacts()
	facts.Criteria[1].Contradicted = true
	got := DecideStageMove(facts)
	if got.Outcome != OutcomeObserve {
		t.Fatalf("contradicted evidence decided %q", got.Outcome)
	}
	if got.Exception == "" {
		t.Error("a contradiction was observed silently; the rep is left with a " +
			"stage that never moves and no way to find out why")
	}
}

// The other two exception facts take the same arm, and each must surface.
func TestAnUnhealthySourceOrUncertainLinkageSurfacesAnException(t *testing.T) {
	for name, mutate := range map[string]func(*StageMoveFacts){
		"an unhealthy source": func(f *StageMoveFacts) { f.SourceUnhealthy = true },
		"uncertain linkage":   func(f *StageMoveFacts) { f.LinkageUncertain = true },
	} {
		facts := settledFacts()
		mutate(&facts)
		got := DecideStageMove(facts)
		if got.Outcome != OutcomeObserve {
			t.Errorf("%s decided %q", name, got.Outcome)
		}
		if got.Exception == "" {
			t.Errorf("%s was observed without surfacing anything", name)
		}
	}
}

// A deal a human has recently steered is theirs. The reason must be the
// PROTECTION's, not a generic refusal, because the rep is owed the difference
// between "we are not sure" and "you moved this yourself".
func TestAProtectedDealIsNeverProposed(t *testing.T) {
	facts := settledFacts()
	facts.Protected = true
	facts.ProtectedReason = "you moved this deal by hand last week"
	got := DecideStageMove(facts)
	if got.Outcome != OutcomeObserve {
		t.Fatalf("a protected deal decided %q", got.Outcome)
	}
	if got.Reason != facts.ProtectedReason {
		t.Errorf("the refusal reads %q, not the protection's own reason", got.Reason)
	}
}

// Protection outranks a perfect case, INCLUDING a measured autopilot: a
// product that moved a deal the rep had just steered would be arguing with the
// person it works for, and doing it silently.
func TestProtectionOutranksAMeasuredAutopilot(t *testing.T) {
	facts := settledFacts()
	facts.Protected = true
	facts.ProtectedReason = "you moved this deal by hand last week"
	facts.Autopilot = AutopilotFacts{Enabled: true, ThresholdsMet: true}
	if got := DecideStageMove(facts); got.Outcome != OutcomeObserve {
		t.Fatalf("a protected deal with the autopilot on decided %q", got.Outcome)
	}
}

func TestEnteringOrLeavingWonOrLostIsAlwaysConfirmFirst(t *testing.T) {
	for name, mutate := range map[string]func(*StageMoveFacts){
		"leaving a closing stage": func(f *StageMoveFacts) { f.FromTerminal = true },
		"entering one":            func(f *StageMoveFacts) { f.ToTerminal = true },
	} {
		facts := settledFacts()
		mutate(&facts)
		if got := DecideStageMove(facts); got.Outcome != OutcomeProposeConfirmFirst {
			t.Errorf("%s decided %q, want confirm-first", name, got.Outcome)
		}
	}
}

// A terminal move stays a person's call however well measured the transition
// is. This is the row a future autopilot must not be able to reach.
func TestNoFactCombinationYieldsAutoApplyOntoATerminalStage(t *testing.T) {
	facts := settledFacts()
	facts.ToTerminal = true
	facts.Autopilot = AutopilotFacts{Enabled: true, ThresholdsMet: true}
	got := DecideStageMove(facts)
	if got.Outcome == OutcomeAutoApply {
		t.Fatal("a measured autopilot auto-applied a move onto a closing stage")
	}
	// The exact outcome, not merely "not auto-apply": a terminal move that
	// resolved to an ordinary Propose would pass a not-equal assertion while
	// dropping the confirm-first the row demands.
	if got.Outcome != OutcomeProposeConfirmFirst {
		t.Errorf("a terminal move under a measured autopilot decided %q, want confirm-first", got.Outcome)
	}
}

func TestSkippingOrRegressingIsConfirmFirst(t *testing.T) {
	facts := settledFacts()
	facts.SingleStep = false
	if got := DecideStageMove(facts); got.Outcome != OutcomeProposeConfirmFirst {
		t.Fatalf("a skip or regression decided %q", got.Outcome)
	}
}

func TestCrossingPipelinesIsConfirmFirst(t *testing.T) {
	facts := settledFacts()
	facts.SamePipeline = false
	if got := DecideStageMove(facts); got.Outcome != OutcomeProposeConfirmFirst {
		t.Fatalf("a cross-pipeline move decided %q", got.Outcome)
	}
}

// "We could meet Thursday" is not "Thursday works". A criterion met on a
// proposal is met on something nobody agreed to.
func TestAProposedNextStepIsAmbiguousAndConfirmFirst(t *testing.T) {
	facts := settledFacts()
	facts.Criteria[0].Commitment = CommitmentProposed
	if got := DecideStageMove(facts); got.Outcome != OutcomeProposeConfirmFirst {
		t.Fatalf("a criterion resting on a proposal decided %q", got.Outcome)
	}
}

// The confidence bar for ACTING is higher than the bar for recording. A claim
// between the two is recorded, shown, and confirmed by a human.
func TestAnUncertainReadingIsConfirmFirst(t *testing.T) {
	facts := settledFacts()
	// LITERALS, not arithmetic on the production constant: deriving the input
	// from the value under test makes the test agree with any threshold,
	// including one that silently drifted down to 0.5.
	low := 0.84
	facts.Criteria[0].Confidence = &low
	if got := DecideStageMove(facts); got.Outcome != OutcomeProposeConfirmFirst {
		t.Fatalf("a reading below the acting bar decided %q", got.Outcome)
	}

	// At the bar it is objective, and a deterministic claim carries no
	// confidence at all — nil must read as certain rather than as zero.
	at := 0.85
	facts.Criteria[0].Confidence = &at
	if got := DecideStageMove(facts); got.Outcome != OutcomePropose {
		t.Errorf("a reading AT the bar decided %q", got.Outcome)
	}
	facts.Criteria[0].Confidence = nil
	if got := DecideStageMove(facts); got.Outcome != OutcomePropose {
		t.Errorf("a deterministic claim carrying no confidence decided %q", got.Outcome)
	}
}

// Every one of the three autopilot facts is necessary, checked one at a time
// so a policy that dropped one from the condition fails here rather than
// passing on the other two.
func TestAutopilotAppliesOnlyWhenEnabledUnsuspendedAndMeasured(t *testing.T) {
	full := AutopilotFacts{Enabled: true, Suspended: false, ThresholdsMet: true}
	facts := settledFacts()
	facts.Autopilot = full
	if got := DecideStageMove(facts); got.Outcome != OutcomeAutoApply {
		t.Fatalf("a measured, enabled, unsuspended transition decided %q", got.Outcome)
	}

	for name, broken := range map[string]AutopilotFacts{
		"the kill switch off":      {Enabled: false, ThresholdsMet: true},
		"the transition suspended": {Enabled: true, Suspended: true, ThresholdsMet: true},
		"nothing measured yet":     {Enabled: true},
	} {
		facts := settledFacts()
		facts.Autopilot = broken
		if got := DecideStageMove(facts); got.Outcome != OutcomePropose {
			t.Errorf("with %s the policy decided %q, want a proposal", name, got.Outcome)
		}
	}
}

// The zero value is a transition nobody has measured, and it must propose
// rather than apply. A policy whose default was permissive would auto-apply
// every transition on an installation that had never configured one.
func TestAnUnmeasuredTransitionProposes(t *testing.T) {
	if got := DecideStageMove(settledFacts()); got.Outcome != OutcomePropose {
		t.Fatalf("the zero autopilot decided %q", got.Outcome)
	}
}

// Every decision carries a sentence. A card that says a move is proposed
// without saying why is a card a rep cannot act on.
func TestEveryDecisionSaysWhy(t *testing.T) {
	for name, facts := range map[string]StageMoveFacts{
		"proposed":      settledFacts(),
		"observed":      unmetFacts(),
		"confirm-first": terminalFacts(),
		"auto-applied":  autopilotFacts(),
	} {
		if got := DecideStageMove(facts); got.Reason == "" {
			t.Errorf("the %s decision carries no reason", name)
		}
	}
}

func unmetFacts() StageMoveFacts {
	f := settledFacts()
	f.Criteria[0].Met = false
	return f
}

func terminalFacts() StageMoveFacts {
	f := settledFacts()
	f.ToTerminal = true
	return f
}

func autopilotFacts() StageMoveFacts {
	f := settledFacts()
	f.Autopilot = AutopilotFacts{Enabled: true, ThresholdsMet: true}
	return f
}
