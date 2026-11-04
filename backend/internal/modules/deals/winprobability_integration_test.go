// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// probabilityInSQL reads the deal's probability through the SQL helpers and
// its stored halves through the Go helper, and fails when the two disagree.
func probabilityInSQL(t *testing.T, e *configEnv, deal ids.DealID) (int, Source) {
	t.Helper()
	var sqlProbability, stageProbability int
	var sqlSource string
	var override *int
	var setBy *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT `+WinProbabilitySQL("d")+`, `+WinProbabilitySourceSQL("d")+`, `+StageWinProbabilitySQL("d")+`,
		        d.win_probability_override, d.win_probability_set_by
		   FROM deal d WHERE d.id = $1`, deal).
		Scan(&sqlProbability, &sqlSource, &stageProbability, &override, &setBy); err != nil {
		t.Fatalf("reading the probability: %v", err)
	}
	goProbability, goSource := EffectiveWinProbability(override, setBy, stageProbability)
	if goProbability != sqlProbability || string(goSource) != sqlSource {
		t.Fatalf("SQL reads %d%% from %s, Go reads %d%% from %s", sqlProbability, sqlSource, goProbability, goSource)
	}
	return sqlProbability, goSource
}

// A probability set by hand wins over the stage until the deal moves; the move
// sets it back to the new stage's and says so.
func TestAStageMoveSetsAHandSetProbabilityBackToTheStage(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, next := seedOwnedDeal(t, e)
	if p, source := probabilityInSQL(t, e, deal); p != 0 || source != SourceStage {
		t.Fatalf("a new deal reads %d%% from %s, want its stage's 0%%", p, source)
	}

	set := 70
	updated, err := e.store.UpdateDeal(e.asDealWriter(), deal, UpdateDealInput{WinProbability: &set})
	if err != nil {
		t.Fatalf("setting the probability: %v", err)
	}
	if *updated.WinProbability != 70 || string(*updated.WinProbabilitySource) != string(SourceHuman) || *updated.StageWinProbability != 0 {
		t.Errorf("the update answered %d%% from %s on a %d%% stage, want 70%% set by hand on 0%%",
			*updated.WinProbability, *updated.WinProbabilitySource, *updated.StageWinProbability)
	}
	if p, source := probabilityInSQL(t, e, deal); p != 70 || source != SourceHuman {
		t.Fatalf("after the edit the deal reads %d%% from %s, want 70%% set by hand", p, source)
	}

	moved, err := e.store.AdvanceDeal(e.asDealWriter(), deal, AdvanceDealInput{ToStageID: next})
	if err != nil {
		t.Fatalf("moving the deal: %v", err)
	}
	if moved.Probability == nil || moved.Probability.WinProbability != 10 || moved.Probability.StageName != "Proposal" ||
		moved.Probability.Note != "Probability set to 10% by the stage Proposal" {
		t.Errorf("the move reported %+v, want 10%% set by the stage Proposal", moved.Probability)
	}
	if p, source := probabilityInSQL(t, e, deal); p != 10 || source != SourceStage {
		t.Errorf("after the move the deal reads %d%% from %s, want the stage's 10%%", p, source)
	}

	again, err := e.store.AdvanceDeal(e.asDealWriter(), deal, AdvanceDealInput{ToStageID: next})
	if err != nil {
		t.Fatalf("repeating the move: %v", err)
	}
	if again.Probability != nil {
		t.Errorf("a move to the stage the deal holds reported %+v, want nothing set", again.Probability)
	}
}

// A won or lost deal takes its stage's probability; a hand-set one is refused.
func TestAClosedDealRefusesAProbabilityOfItsOwn(t *testing.T) {
	e := setupConfigEnv(t)
	deal, from, _ := seedOwnedDeal(t, e)
	var pipeline ids.PipelineID
	if err := e.owner.QueryRow(context.Background(), `SELECT pipeline_id FROM stage WHERE id = $1`, from).Scan(&pipeline); err != nil {
		t.Fatalf("reading the pipeline: %v", err)
	}
	zero := 0
	lost, err := e.store.CreateStage(e.as(), CreateStageInput{
		PipelineID: pipeline, Name: "Lost", Position: 9, Semantic: string(SemanticLost), WinProbability: &zero,
	})
	if err != nil {
		t.Fatalf("seeding the lost stage: %v", err)
	}
	reason := "went with a competitor"
	if _, err := e.store.AdvanceDeal(e.asDealWriter(), deal, AdvanceDealInput{
		ToStageID: ids.From[ids.StageKind](ids.UUID(lost.Id)), LostReason: &reason,
	}); err != nil {
		t.Fatalf("losing the deal: %v", err)
	}

	set := 40
	_, err = e.store.UpdateDeal(e.asDealWriter(), deal, UpdateDealInput{WinProbability: &set})
	if _, ok := errors.AsType[*ProbabilityFixedWhenClosedError](err); !ok {
		t.Fatalf("setting a probability on a lost deal answered %v, want probability_fixed_when_closed", err)
	}
	if p, source := probabilityInSQL(t, e, deal); p != 0 || source != SourceStage {
		t.Errorf("the lost deal reads %d%% from %s, want the stage's 0%%", p, source)
	}
}
