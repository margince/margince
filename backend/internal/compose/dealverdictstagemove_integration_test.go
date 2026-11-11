// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Margince moves an open deal to a later stage when the cited evidence meets
// the exit criteria of the stage it leaves. Undo moves it back. Only the model
// is a stand-in.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
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
	return setupStageMoveLedger(t, true)
}

// setupStageMoveLedger is setupStageMove, with the ledger recording the
// criterion met only when ledger is set.
func setupStageMoveLedger(t *testing.T, ledger bool) *stageMoveEnv {
	t.Helper()
	v := setupDealVerdict(t)
	next := nextOpenStage(t, v, v.stage)
	criterion, err := v.Deals.CreateStageExitCriterion(v.Admin(), deals.CreateCriterionInput{
		StageID: v.stage, Key: "budget_confirmed", Label: "Budget confirmed", Kind: string(deals.CriterionCustom),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ledger {
		recordLedger(t, v, criterion.Id.String(), v.note)
	}
	move := &stageMoveLane{note: v.note, to: next.String(), criterion: criterion.Id.String(), quote: "The board agreed"}
	v.pass = newDealVerdictPass(v.Pool, dealVerdictRefs(move), slog.New(slog.DiscardHandler))
	v.pass.now = func() time.Time { return v.now.Add(time.Minute) }
	return &stageMoveEnv{dealVerdictEnv: v, next: next, move: move}
}

// recordLedger records the criterion met on the activity through the
// ledger's own writer, as the evidence pass does.
func recordLedger(t *testing.T, v *dealVerdictEnv, criterion, activity string) {
	t.Helper()
	criterionID, err := ids.ParseAs[ids.ExitCriterionKind](criterion)
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := ids.Parse(activity)
	if err != nil {
		t.Fatal(err)
	}
	confidence, quote := 0.9, "The board agreed"
	if _, err := v.Deals.RecordStageEvidence(v.Admin(), deals.EvidenceInput{
		DealID: ids.From[ids.DealKind](v.deal), CriterionID: criterionID,
		SourceType: deals.SourceActivity, SourceID: activityID, Snippet: &quote,
		AuthorSide: deals.AuthorBuyer, Commitment: deals.CommitmentAgreed, Met: true,
		Confidence: &confidence, ObservedAt: v.now, ExtractedBy: stageEvidenceExtractedBy,
	}); err != nil {
		t.Fatal(err)
	}
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

func TestAnAdminsStoredOffStopsEveryStageMove(t *testing.T) {
	v := setupStageMove(t)
	t.Cleanup(func() {
		if _, err := v.Pool.Exec(context.Background(), `DELETE FROM setting WHERE key = $1`, deals.StageAutopilotEnabled.Key()); err != nil {
			t.Errorf("restoring the default: %v", err)
		}
	})
	if _, err := v.Pool.Exec(t.Context(), `INSERT INTO setting (key, value) VALUES ($1, 'false'::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, deals.StageAutopilotEnabled.Key()); err != nil {
		t.Fatal(err)
	}
	v.run(t)
	if got := v.field(t, "stage_id"); got != v.stage.String() {
		t.Fatalf("stage = %s, want it left at %s while moves are off", got, v.stage)
	}
}

func TestAnAdminsOffBetweenTheReadAndTheWriteStopsTheMove(t *testing.T) {
	v := setupStageMove(t)
	t.Cleanup(func() {
		if _, err := v.Pool.Exec(context.Background(), `DELETE FROM setting WHERE key = $1`, deals.StageAutopilotEnabled.Key()); err != nil {
			t.Errorf("restoring the default: %v", err)
		}
	})
	v.move.during = func() {
		if _, err := v.Pool.Exec(context.Background(), `INSERT INTO setting (key, value) VALUES ($1, 'false'::jsonb)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, deals.StageAutopilotEnabled.Key()); err != nil {
			t.Error(err)
		}
	}
	v.run(t)
	if got := v.outcomes(t); got != "move_stage="+deals.VerdictDroppedSwitch {
		t.Fatalf("outcomes = %s, want the move stopped by the switch turned off mid-read", got)
	}
}

// buyerCriterion puts a buyer milestone on the deal's stage in place of the
// custom one, and cites the activity for it.
func (v *stageMoveEnv) buyerCriterion(t *testing.T, kind deals.CriterionKind, activity ids.UUID, quote string) {
	t.Helper()
	criterion, err := v.Deals.CreateStageExitCriterion(v.Admin(), deals.CreateCriterionInput{
		StageID: v.stage, Key: "proof_" + string(kind), Label: "Proof", Kind: string(kind),
	})
	if err != nil {
		t.Fatal(err)
	}
	v.WsExec(t, `UPDATE stage_exit_criterion SET archived_at = now() WHERE stage_id = $1 AND id <> $2`, v.stage, criterion.Id)
	v.move.note, v.move.criterion, v.move.quote = activity.String(), criterion.Id.String(), quote
}

// seedDealActivity writes one activity on the deal with a sender, as a
// mailbox sync leaves it.
func (v *stageMoveEnv) seedDealActivity(t *testing.T, kind, direction, from, status, body string) ids.UUID {
	t.Helper()
	owner := integration.OwnerConn(t)
	id := ids.NewV7()
	var meetingStatus, dir *string
	if status != "" {
		meetingStatus = &status
	}
	if direction != "" {
		dir = &direction
	}
	if _, err := owner.Exec(context.Background(), `INSERT INTO activity
		(id, kind, subject, body, occurred_at, created_at, direction, meeting_status, source, captured_by)
		VALUES ($1, $2, 'Contract', $3, $4, $4, $5, $6, 'manual', 'human:x')`,
		id, kind, body, v.now, dir, meetingStatus); err != nil {
		t.Fatal(err)
	}
	integration.LinkActivity(t, owner, id, "deal", v.deal)
	if from != "" {
		if _, err := owner.Exec(context.Background(),
			`INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'from')`, id, from); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// seedBuyer makes a stakeholder of the deal who writes from address.
func (v *stageMoveEnv) seedBuyer(t *testing.T, address string) {
	t.Helper()
	owner := integration.OwnerConn(t)
	contact := integration.SeedStakeholder(t, v.Env, owner, v.deal)
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, contact, address); err != nil {
		t.Fatal(err)
	}
}

func TestABuyerMilestoneNeedsMailFromABuyerOfTheDeal(t *testing.T) {
	for _, tc := range []struct {
		name, from   string
		moves, moved bool
	}{
		{"the deal's stakeholder wrote it", "buyer@customer.example", true, false},
		{"somebody who is no contact of the deal wrote it", "stranger@elsewhere.example", false, false},
		{"the address was another contact's before the stakeholder's", "buyer@customer.example", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := setupStageMove(t)
			v.seedBuyer(t, "buyer@customer.example")
			if tc.moved {
				v.WsExec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by, archived_at)
					VALUES ($1, 'buyer@customer.example', 'manual', 'human:x', now())`, v.SeedContact(t, "Former Holder", &v.Rep1))
			}
			mail := v.seedDealActivity(t, "email", "inbound", tc.from, "", "Please send us the contract draft.")
			v.buyerCriterion(t, deals.CriterionBuyerConfirmed, mail, "send us the contract draft")
			v.run(t)
			moved := v.field(t, "stage_id") == v.next.String()
			if moved != tc.moves {
				t.Fatalf("moved = %t, want %t (outcomes %s)", moved, tc.moves, v.outcomes(t))
			}
		})
	}
}

func TestAMeetingThatWasNotHeldMeetsNoHeldEvent(t *testing.T) {
	v := setupStageMove(t)
	meeting := v.seedDealActivity(t, "meeting", "", "", "canceled", "Demo with the buyer, cancelled by them.")
	v.buyerCriterion(t, deals.CriterionEventHeld, meeting, "Demo with the buyer")
	v.run(t)
	if got := v.field(t, "stage_id"); got != v.stage.String() {
		t.Fatalf("stage = %s, want it left at %s (outcomes %s)", got, v.stage, v.outcomes(t))
	}
	if got := v.outcomes(t); got != "move_stage="+deals.VerdictDroppedStageRefused {
		t.Fatalf("outcomes = %s, want the row lock to refuse the cancelled meeting", got)
	}
}

func TestACustomCriterionTheLedgerDoesNotRecordMovesNothing(t *testing.T) {
	v := setupStageMoveLedger(t, false)
	v.run(t)
	if got := v.field(t, "stage_id"); got != v.stage.String() {
		t.Fatalf("stage = %s, want it left at %s: only the quote matched (outcomes %s)", got, v.stage, v.outcomes(t))
	}
}

func TestAStageThatAsksForNothingGetsNoMove(t *testing.T) {
	v := setupStageMove(t)
	v.WsExec(t, `UPDATE stage_exit_criterion SET archived_at = now() WHERE stage_id = $1`, v.stage)
	v.move.criterion = ""
	v.run(t)
	if got := v.outcomes(t); got != "move_stage="+deals.VerdictDroppedNoCriteria {
		t.Fatalf("outcomes = %s, want the move refused for a stage with no exit criteria", got)
	}
}

func TestAStageChangeBetweenTheReadAndTheWriteStopsTheMove(t *testing.T) {
	cases := map[string]func(v *stageMoveEnv){
		"the stages were reordered": func(v *stageMoveEnv) {
			order := strings.Split(v.WsScalar(t, `SELECT string_agg(id::text, ',' ORDER BY position) FROM stage
				WHERE pipeline_id = $1 AND archived_at IS NULL`, v.pipeline), ",")
			for i, id := range order {
				switch id {
				case v.stage.String():
					order[i] = v.next.String()
				case v.next.String():
					order[i] = v.stage.String()
				}
			}
			stageIDs := make([]ids.UUID, 0, len(order))
			for _, id := range order {
				parsed, err := ids.Parse(id)
				if err != nil {
					t.Error(err)
					return
				}
				stageIDs = append(stageIDs, parsed)
			}
			if _, err := v.Deals.ReorderStages(v.Admin(), v.pipeline, stageIDs, nil); err != nil {
				t.Error(err)
			}
		},
		"a criterion became required": func(v *stageMoveEnv) {
			if _, err := v.Deals.CreateStageExitCriterion(v.Admin(), deals.CreateCriterionInput{
				StageID: v.stage, Key: "signed_nda", Label: "NDA signed", Kind: string(deals.CriterionCustom),
			}); err != nil {
				t.Error(err)
			}
		},
		"the cited note was unlinked": func(v *stageMoveEnv) {
			v.WsExec(t, `DELETE FROM activity_link WHERE deal_id = $1 AND activity_id = $2`, v.deal, v.note)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			v := setupStageMove(t)
			v.move.during = func() { change(v) }
			v.run(t)
			if got := v.field(t, "stage_id"); got == v.next.String() {
				t.Fatalf("the deal moved although %s while the model read (outcomes %s)", name, v.outcomes(t))
			}
		})
	}
}
