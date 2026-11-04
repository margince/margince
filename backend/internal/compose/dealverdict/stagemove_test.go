// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
)

const (
	negotiationID = "11111111-1111-4111-8111-111111111111"
	contractID    = "22222222-2222-4222-8222-222222222222"
	askedID       = "33333333-3333-4333-8333-333333333333"
	demoHeldID    = "44444444-4444-4444-8444-444444444444"
	mailID        = "66666666-6666-4666-8666-666666666666"
	oursID        = "77777777-7777-4777-8777-777777777777"
	demoID        = "88888888-8888-4888-8888-888888888888"
	callID        = "99999999-9999-4999-8999-999999999999"
)

// moveInput is a deal in Proposal (position 1) with Negotiation and Contract
// after it. Leaving Proposal needs the buyer to ask for the contract; leaving
// Negotiation needs a demo held.
func moveInput() Input {
	in := guardInput()
	in.StagePosition = 1
	in.Later = []StageOption{
		{ID: negotiationID, Name: "Negotiation", Position: 2, Probability: 75},
		{ID: contractID, Name: "Contract", Position: 3, Probability: 90},
	}
	in.ExitCriteria = []ExitCriterion{
		{ID: askedID, StagePosition: 1, Stage: "Proposal", Label: "Buyer asked for the contract", Kind: "buyer_confirmed"},
		{ID: demoHeldID, StagePosition: 2, Stage: "Negotiation", Label: "Demo held", Kind: "event_held"},
	}
	in.Activities = append(in.Activities,
		Activity{ID: oursID, Kind: "email", Direction: "outbound", OccurredAt: today.AddDate(0, 0, -2),
			ArrivedAt: today.AddDate(0, 0, -2), Body: "They asked us for the contract draft."},
		Activity{ID: callID, Kind: callID, OccurredAt: today.AddDate(0, 0, -1),
			ArrivedAt: today.AddDate(0, 0, -1), Body: "We talked it through."},
		Activity{ID: demoID, Kind: "meeting", OccurredAt: today.AddDate(0, 0, -1),
			ArrivedAt: today.AddDate(0, 0, -1), Body: "Demo with the buyer went well."},
	)
	in.Activities[0].ID = mailID
	in.Activities[0].Body += " Please send us the contract draft."
	return in
}

func moveState() State {
	state := guardState()
	state.SwitchedOn[deals.VerdictStageMoveCorrection] = true
	return state
}

func moveAction(to string, cites ...CriterionCite) Action {
	cited := []string{mailID}
	for _, cite := range cites {
		cited = append(cited, cite.ActivityID)
	}
	return Action{Kind: KindMoveStage, StageID: to, Criteria: cites, Cited: cited, Confidence: 0.9, Reason: "r"}
}

var (
	buyerAsked = CriterionCite{CriterionID: askedID, ActivityID: mailID, QuotedWords: "please send us the contract draft"}
	demoHeld   = CriterionCite{CriterionID: demoHeldID, ActivityID: demoID, QuotedWords: "Demo with the buyer"}
)

func TestAStageMoveNeedsEveryCriterionOfTheStagesItLeaves(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		action Action
		want   string
	}{
		{"the next stage with its criterion met", moveAction(negotiationID, buyerAsked), OutcomeApply},
		{"the next stage with no criterion cited", moveAction(negotiationID), DroppedCriterionUnmet},
		{"a skip with both stages' criteria met", moveAction(contractID, buyerAsked, demoHeld), OutcomeApply},
		{"a skip missing the skipped stage's criterion", moveAction(contractID, buyerAsked), DroppedCriterionUnmet},
		{"a buyer milestone resting on our own mail", moveAction(negotiationID,
			CriterionCite{CriterionID: askedID, ActivityID: oursID, QuotedWords: "asked us for the contract draft"}), DroppedCriterionUnmet},
		{"a quote the cited activity does not hold", moveAction(negotiationID,
			CriterionCite{CriterionID: askedID, ActivityID: mailID, QuotedWords: "we accept your terms"}), DroppedCriterionUnmet},
		{"a held event resting on a call", moveAction(contractID, buyerAsked,
			CriterionCite{CriterionID: demoHeldID, ActivityID: callID, QuotedWords: "talked it through"}), DroppedCriterionUnmet},
		{"a held event resting on a mail", moveAction(contractID, buyerAsked,
			CriterionCite{CriterionID: demoHeldID, ActivityID: mailID, QuotedWords: "start in March"}), DroppedCriterionUnmet},
		{"a stage that is not a later open one", moveAction("55555555-5555-4555-8555-555555555555", buyerAsked), DroppedNotForward},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Decide(moveInput(), moveState(), Payload{Actions: []Action{tc.action}}, fixedRates{})
			if err != nil {
				t.Fatal(err)
			}
			if got[0].Outcome != tc.want {
				t.Fatalf("outcome = %q, want %q", got[0].Outcome, tc.want)
			}
		})
	}
}

func TestAnAppliedMoveCarriesItsTargetAndTheCriteriaItCovers(t *testing.T) {
	t.Parallel()
	got, err := Decide(moveInput(), moveState(), Payload{Actions: []Action{moveAction(contractID, buyerAsked, demoHeld)}}, fixedRates{})
	if err != nil {
		t.Fatal(err)
	}
	change := got[0].Change
	if change == nil || change.ToStageID.String() != contractID || len(change.CriteriaMet) != 2 {
		t.Fatalf("change = %+v, want a move to Contract covering both criteria", change)
	}
	if change.Correction != deals.VerdictStageMoveCorrection {
		t.Fatalf("correction = %q, want %q", change.Correction, deals.VerdictStageMoveCorrection)
	}
}

func TestAKindSettledByItsOwnRecordOrUnknownBlocksTheMove(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{
		string(deals.CriterionLegalReviewed), string(deals.CriterionDocumentSigned),
		string(deals.CriterionRoleIdentified), "a_kind_nobody_knows",
	} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			in := moveInput()
			in.ExitCriteria[0].Kind = kind
			got, err := Decide(in, moveState(), Payload{Actions: []Action{moveAction(negotiationID, buyerAsked)}}, fixedRates{})
			if err != nil {
				t.Fatal(err)
			}
			if got[0].Outcome != DroppedCriterionUnmet {
				t.Fatalf("outcome = %q, want %q", got[0].Outcome, DroppedCriterionUnmet)
			}
		})
	}
}

func TestAColleaguesStageMoveAfterTheEvidenceWins(t *testing.T) {
	t.Parallel()
	state := moveState()
	state.HumanEditedAt = map[string]time.Time{deals.VerdictStageMoveCorrection: today}
	got, err := Decide(moveInput(), state, Payload{Actions: []Action{moveAction(negotiationID, buyerAsked)}}, fixedRates{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Outcome != DroppedHumanEdit {
		t.Fatalf("outcome = %q, want %q", got[0].Outcome, DroppedHumanEdit)
	}
}

func TestTheValidatorRefusesAMoveToAStageNotListed(t *testing.T) {
	t.Parallel()
	in := moveInput()
	action := moveAction("55555555-5555-4555-8555-555555555555")
	if msg := validateMove(action, in); msg == "" {
		t.Fatal("a move to a stage not listed passed the validator")
	}
	action = moveAction(negotiationID, CriterionCite{CriterionID: askedID, ActivityID: demoID, QuotedWords: "x"})
	action.Cited = []string{mailID}
	if msg := validateMove(action, in); msg == "" {
		t.Fatal("a criterion resting on an activity the move does not cite passed the validator")
	}
}
