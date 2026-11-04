// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Margince moves an open deal to a later stage when the cited evidence meets
// the exit criteria of the stage it leaves, and Undo moves it back. The pass,
// the stores and the stage gates are the real ones; only the model is a
// stand-in.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// stageMoveLane answers every deal with one move, citing the test's note for
// the stage's exit criterion when criterion is set.
type stageMoveLane struct {
	note, to, criterion string
	quote               string
	during              func()
}

func (l *stageMoveLane) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	if l.during != nil {
		l.during()
	}
	criteria := "[]"
	if l.criterion != "" {
		criteria = fmt.Sprintf(`[{"criterion_id":%q,"activity_id":%q,"quoted_words":%q}]`, l.criterion, l.note, l.quote)
	}
	return model.Response{Text: fmt.Sprintf(`{"actions":[
		{"kind":"move_stage","date":"","amount":0,"currency":"","quoted_words":"","probability":0,
		 "stage_id":%q,"criteria":%s,"cited_activity_ids":[%q],
		 "reason":"They asked for the contract draft","confidence":0.9}]}`, l.to, criteria, l.note)}, nil
}

type stageMoveEnv struct {
	*dealVerdictEnv
	next ids.StageID
	move *stageMoveLane
}

// setupStageMove is a deal in the pipeline's first open stage, whose exit
// criterion the note meets in its own words.
func setupStageMove(t *testing.T) *stageMoveEnv {
	t.Helper()
	v := setupDealVerdict(t)
	next := nextOpenStage(t, v, v.stage)
	criterion, err := v.Deals.CreateStageExitCriterion(v.Admin(), deals.CreateCriterionInput{
		StageID: v.stage, Key: "budget_confirmed", Label: "Budget confirmed", Kind: string(deals.CriterionCustom),
	})
	if err != nil {
		t.Fatal(err)
	}
	move := &stageMoveLane{note: v.note, to: next.String(), criterion: criterion.Id.String(), quote: "The board agreed"}
	v.pass = newDealVerdictPass(v.Pool, move, slog.New(slog.DiscardHandler))
	v.pass.now = func() time.Time { return v.now.Add(time.Minute) }
	return &stageMoveEnv{dealVerdictEnv: v, next: next, move: move}
}

// nextOpenStage is the open stage after from in its pipeline.
func nextOpenStage(t *testing.T, v *dealVerdictEnv, from ids.StageID) ids.StageID {
	t.Helper()
	next, err := ids.ParseAs[ids.StageKind](v.WsScalar(t, `SELECT st.id FROM stage st JOIN stage cur ON cur.id = $1
		WHERE st.pipeline_id = cur.pipeline_id AND st.semantic = 'open' AND st.position > cur.position
		ORDER BY st.position LIMIT 1`, from))
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestMarginceMovesADealForwardAndUndoMovesItBack(t *testing.T) {
	v := setupStageMove(t)
	v.run(t)

	if got := v.field(t, "stage_id"); got != v.next.String() {
		t.Fatalf("stage = %s, want the next stage %s (outcomes %s)", got, v.next, v.outcomes(t))
	}
	if n := v.WsCount(t, `SELECT count(*) FROM deal_correction c JOIN deal_stage_history h ON h.id = c.stage_history_id
		WHERE c.deal_id = $1 AND c.correction = $2 AND h.to_stage_id = $3 AND h.changed_by = $4`,
		v.deal, deals.VerdictStageMoveCorrection, v.next, marginceActorID); n != 1 {
		t.Fatalf("stage-move corrections naming Margince's history row = %d, want 1", n)
	}
	changes, _, err := v.Deals.DealChanges(v.owner(t), ids.From[ids.DealKind](v.deal), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want the one move", len(changes))
	}
	summary := stageMoveReceipt(changes[0]).Summary
	if !strings.Contains(summary, "They asked for the contract draft") || !strings.Contains(summary, "Probability now") {
		t.Fatalf("receipt = %q, want the reason and the stage's probability", summary)
	}

	v.undo(t, deals.VerdictStageMoveCorrection)
	if got := v.field(t, "stage_id"); got != v.stage.String() {
		t.Fatalf("stage after Undo = %s, want the stage Margince moved it from %s", got, v.stage)
	}
	if n := v.WsCount(t, `SELECT count(*) FROM deal_stage_history h JOIN deal_correction c ON c.stage_history_id = h.reversal_of
		WHERE c.deal_id = $1 AND c.correction = $2 AND c.reversed_at IS NOT NULL`, v.deal, deals.VerdictStageMoveCorrection); n != 1 {
		t.Fatalf("history rows reversing Margince's move = %d, want 1", n)
	}

	// Nothing new arrived since the Undo, so the same evidence moves nothing.
	v.run(t)
	if got := v.field(t, "stage_id"); got != v.stage.String() {
		t.Fatalf("stage after the next pass = %s, want it held at %s (outcomes %s)", got, v.stage, v.outcomes(t))
	}
	if got := v.outcomes(t); !strings.Contains(got, "move_stage=dropped:undo_hold") {
		t.Fatalf("outcomes = %s, want the move held back by the Undo", got)
	}
}

func TestAMoveWhoseCriterionIsNotCitedChangesNothing(t *testing.T) {
	v := setupStageMove(t)
	v.move.criterion = ""
	v.run(t)
	if got := v.field(t, "stage_id"); got != v.stage.String() {
		t.Fatalf("stage = %s, want it left at %s", got, v.stage)
	}
	if got := v.outcomes(t); got != "move_stage=dropped:criterion_unmet" {
		t.Fatalf("outcomes = %s, want the unmet criterion named", got)
	}
}

func TestAColleaguesMoveWhileTheModelReadsStopsTheMove(t *testing.T) {
	v := setupStageMove(t)
	v.move.during = func() {
		if _, err := v.Deals.AdvanceDeal(v.owner(t), ids.From[ids.DealKind](v.deal), deals.AdvanceDealInput{ToStageID: v.next}); err != nil {
			t.Fatal(err)
		}
	}
	v.run(t)
	if n := v.WsCount(t, `SELECT count(*) FROM deal_correction WHERE deal_id = $1`, v.deal); n != 0 {
		t.Fatalf("corrections = %d, want none: the colleague's move came after the read", n)
	}
}

func TestAnUndoAfterAColleagueMovedTheDealOnIsRefused(t *testing.T) {
	v := setupStageMove(t)
	v.run(t)
	later := nextOpenStage(t, v.dealVerdictEnv, v.next)
	if _, err := v.Deals.AdvanceDeal(v.owner(t), ids.From[ids.DealKind](v.deal), deals.AdvanceDealInput{ToStageID: later}); err != nil {
		t.Fatal(err)
	}
	correction, err := ids.Parse(v.WsScalar(t, `SELECT id FROM deal_correction WHERE deal_id = $1 AND correction = $2`,
		v.deal, deals.VerdictStageMoveCorrection))
	if err != nil {
		t.Fatal(err)
	}
	_, err = v.Deals.RevertCorrection(v.owner(t), correction, nil, nil)
	if conflict, ok := errors.AsType[*deals.CorrectionReversalError](err); !ok || !strings.Contains(conflict.Reason, "moved since") {
		t.Fatalf("Undo after a colleague's move = %v, want the moved-since refusal", err)
	}
	if got := v.field(t, "stage_id"); got != later.String() {
		t.Fatalf("stage = %s, want the colleague's %s kept", got, later)
	}
}
