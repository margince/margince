// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// Reading a real deal into the facts the policy judges.
//
// The policy itself is unit-tested per decision row; what these prove is that
// the SQL under it answers what those rows assume — which criterion counts as
// met, which evidence wins when two disagree, and which of the three
// protections fired.

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// twoStagePipeline seeds a pipeline with two open stages and a deal on the
// first, and answers the deal plus both stage ids.
func twoStagePipeline(t *testing.T, e *configEnv) (ids.DealID, ids.StageID, ids.StageID) {
	t.Helper()
	ctx := e.as()
	pipeline, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: "Sales " + ids.NewV7().String()})
	if err != nil {
		t.Fatalf("seeding a pipeline: %v", err)
	}
	pipelineID := ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
	open := 0
	first, err := e.store.CreateStage(ctx, CreateStageInput{
		PipelineID: pipelineID, Name: "Discovery", Position: 0,
		Semantic: string(SemanticOpen), WinProbability: &open,
	})
	if err != nil {
		t.Fatalf("seeding the first stage: %v", err)
	}
	second, err := e.store.CreateStage(ctx, CreateStageInput{
		PipelineID: pipelineID, Name: "Negotiation", Position: 1,
		Semantic: string(SemanticOpen), WinProbability: &open,
	})
	if err != nil {
		t.Fatalf("seeding the second stage: %v", err)
	}
	fromID := ids.From[ids.StageKind](ids.UUID(first.Id))
	owner := ids.From[ids.UserKind](e.admin)
	deal, err := e.store.CreateDeal(e.asDealWriter(), CreateDealInput{
		Name: "Warehouse rollout", PipelineID: pipelineID, StageID: fromID,
		OwnerID: &owner, OwnerExact: true,
	})
	if err != nil {
		t.Fatalf("seeding a deal: %v", err)
	}
	return ids.From[ids.DealKind](ids.UUID(deal.Id)), fromID,
		ids.From[ids.StageKind](ids.UUID(second.Id))
}

// facts reads the deal through the real assembler.
//
// The autopilot is the ZERO value throughout: a transition nobody has measured
// is what every deal starts as, and the autopilot's own arms are unit-tested
// per row. What these prove is the SQL beneath the decision.
func facts(t *testing.T, e *configEnv, dealID ids.DealID) StageProgressionFacts {
	t.Helper()
	var out StageProgressionFacts
	if err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
		var err error
		out, err = ReadStageProgressionFacts(e.asDealWriter(), tx, dealID, AutopilotFacts{}, time.Now())
		return err
	}); err != nil {
		t.Fatalf("reading the deal's progression facts: %v", err)
	}
	return out
}

// The target is the pipeline's next open stage, computed rather than supplied:
// a proposer that could be told where to move a deal is one whose target a bug
// could choose.
func TestTheTargetIsTheNextOpenStageOfTheDealsOwnPipeline(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)

	got := facts(t, e, dealID)
	if got.FromStageID != fromID {
		t.Errorf("the move starts at %v, want the deal's own stage %v", got.FromStageID, fromID)
	}
	if got.ToStageID != toID {
		t.Errorf("the move targets %v, want the next open stage %v", got.ToStageID, toID)
	}
	if got.ToName != "Negotiation" {
		t.Errorf("the target is named %q", got.ToName)
	}
}

// A deal on the last stage has nowhere to go, and that is an ordinary outcome
// rather than a failure.
func TestADealOnTheLastStageHasNothingToPropose(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, _, toID := twoStagePipeline(t, e)
	if _, err := e.store.AdvanceDeal(e.asDealWriter(), dealID,
		AdvanceDealInput{ToStageID: toID}); err != nil {
		t.Fatalf("advancing the deal to the last stage: %v", err)
	}

	err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
		_, err := ReadStageProgressionFacts(e.asDealWriter(), tx, dealID, AutopilotFacts{}, time.Now())
		return err
	})
	if !errors.Is(err, ErrNoNextStage) {
		t.Fatalf("a deal on the last stage answered %v, want the no-next-stage outcome", err)
	}
}

// A criterion with no evidence at all is not met, and the stage's own required
// flag is what decides whether that blocks the move.
func TestACriterionWithNoEvidenceIsNotMet(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, _ := twoStagePipeline(t, e)
	addCriterion(t, e, fromID, "problem_confirmed")

	got := facts(t, e, dealID)
	if len(got.Criteria) != 1 {
		t.Fatalf("read %d criteria, want the one the stage carries", len(got.Criteria))
	}
	if got.Criteria[0].Met {
		t.Error("a criterion nothing has said anything about reads as met")
	}
	if got.Decision.Outcome != OutcomeObserve {
		t.Errorf("an unmet required criterion decided %q", got.Decision.Outcome)
	}
}

// REFUTED evidence does not count. A claim a human struck out is one they said
// was wrong, and counting it would make the correction do nothing.
func TestRefutedEvidenceDoesNotSettleACriterion(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, _ := twoStagePipeline(t, e)
	criterion := addCriterion(t, e, fromID, "problem_confirmed")
	criterionID := ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id))

	recorded, err := e.store.RecordStageEvidence(e.asDealWriter(),
		claim(dealID, criterionID, AuthorBuyer))
	if err != nil {
		t.Fatalf("recording evidence: %v", err)
	}
	if got := facts(t, e, dealID); !got.Criteria[0].Met {
		t.Fatal("precondition: the criterion is not met before the refutation, " +
			"so this test would pass without one")
	}

	if _, err := e.store.RefuteStageEvidence(e.asDealWriter(), dealID,
		ids.UUID(recorded.Id)); err != nil {
		t.Fatalf("refuting: %v", err)
	}
	got := facts(t, e, dealID)
	if got.Criteria[0].Met {
		t.Error("refuted evidence still settles the criterion, so striking a " +
			"claim out changes nothing")
	}
}

// The BUYER's own word wins where two claims settle one criterion. The
// authorship rule is the bar, so a seller-authored row sitting beside a
// buyer's must not be the one the policy judges.
func TestTheBuyersOwnWordWinsAmongClaimsForOneCriterion(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, _ := twoStagePipeline(t, e)
	// A CUSTOM criterion, because the ranking is what is under test: a
	// buyer-milestone kind refuses the seller's claim at the write, and the
	// test would then be about that refusal instead.
	criterion, err := e.store.CreateStageExitCriterion(e.as(), CreateCriterionInput{
		StageID: fromID, Key: "internal_review", Label: "Internal review done",
		Kind: string(CriterionCustom),
	})
	if err != nil {
		t.Fatalf("adding a custom criterion: %v", err)
	}
	criterionID := ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id))

	// THE BUYER'S CLAIM IS THE OLDER ONE, which is what makes this test load-
	// bearing: written first and observed a week earlier, so a query that
	// simply took the most recent row would pick the seller's and this would
	// fail. Authorship priority is the only rule that answers buyer here.
	buyer := claim(dealID, criterionID, AuthorBuyer)
	buyer.SourceID = ids.NewV7()
	buyer.ObservedAt = time.Now().UTC().Add(-7 * 24 * time.Hour)
	if _, err := e.store.RecordStageEvidence(e.asDealWriter(), buyer); err != nil {
		t.Fatalf("recording the buyer's claim: %v", err)
	}
	seller := claim(dealID, criterionID, AuthorSeller)
	seller.SourceID = ids.NewV7()
	seller.ExtractedBy = "stage_evidence_extract"
	confidence := 0.9
	seller.Confidence = &confidence
	if _, err := e.store.RecordStageEvidence(e.asDealWriter(), seller); err != nil {
		t.Fatalf("recording the seller's claim: %v", err)
	}

	got := facts(t, e, dealID)
	if got.Criteria[0].AuthorSide != AuthorBuyer {
		t.Errorf("the criterion reads as %s-authored with a buyer's claim on it",
			got.Criteria[0].AuthorSide)
	}
	if len(got.Criteria[0].EvidenceIDs) != 2 {
		t.Errorf("the card cites %d rows, want both claims so a reader can see "+
			"what the criterion rests on", len(got.Criteria[0].EvidenceIDs))
	}
}

// A human's own stage move in the window protects the deal: a proposal the day
// after is the product arguing with the person it works for.
func TestAHumanStageMoveInTheLastFortnightBlocksAProposal(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	settleEveryCriterion(t, e, dealID, fromID)

	if got := facts(t, e, dealID); got.Decision.Outcome != OutcomePropose {
		t.Fatalf("precondition: a settled deal decided %q, so the protection "+
			"below would not be what this test measures", got.Decision.Outcome)
	}
	// The rep moves it back by hand, which writes a human stage-history row.
	if _, err := e.store.AdvanceDeal(e.asDealWriter(), dealID,
		AdvanceDealInput{ToStageID: toID}); err != nil {
		t.Fatalf("the human stage move: %v", err)
	}
	if _, err := e.store.AdvanceDeal(e.asDealWriter(), dealID,
		AdvanceDealInput{ToStageID: fromID}); err != nil {
		t.Fatalf("moving back: %v", err)
	}

	got := facts(t, e, dealID)
	if got.Decision.Outcome != OutcomeObserve {
		t.Fatalf("a deal a human just moved decided %q", got.Decision.Outcome)
	}
	if got.Decision.Reason == "" {
		t.Error("the protection does not say why, so the rep cannot tell it " +
			"from an ordinary refusal")
	}
}

// settleEveryCriterion gives the deal's current stage one criterion and
// settles it with the buyer's own word.
func settleEveryCriterion(t *testing.T, e *configEnv, dealID ids.DealID, stageID ids.StageID) {
	t.Helper()
	criterion := addCriterion(t, e, stageID, "problem_confirmed")
	if _, err := e.store.RecordStageEvidence(e.asDealWriter(), claim(dealID,
		ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id)), AuthorBuyer)); err != nil {
		t.Fatalf("settling the criterion: %v", err)
	}
}

// Creating a deal is not steering it.
//
// A deal's creation writes a stage-history row like any move, and counting it
// as a human's would protect EVERY new deal for a fortnight — precisely the
// window in which its first criteria get settled. The feature would have
// looked like it simply never fired.
func TestCreatingADealIsNotAHumanStageMove(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, _ := twoStagePipeline(t, e)
	settleEveryCriterion(t, e, dealID, fromID)

	got := facts(t, e, dealID)
	if got.Decision.Outcome != OutcomePropose {
		t.Fatalf("a freshly created, fully settled deal decided %q (%s)",
			got.Decision.Outcome, got.Decision.Reason)
	}
}
