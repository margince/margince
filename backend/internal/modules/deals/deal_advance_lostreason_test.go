// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A reason for a loss must not outlive the loss. The report that counts why
// deals are lost reads this column, so a reason left behind by a re-decided
// deal answers that question with an outcome that no longer stands.
//
// The amount is left nil throughout: a closing deal with an amount freezes an
// FX rate, which needs a transaction, and the reason-clearing this proves is
// decided before any of that.
func TestALostReasonIsClearedOnEveryLandingThatIsNotLost(t *testing.T) {
	reason := "went with a competitor"
	lost := crmcontracts.Deal{
		Status:     crmcontracts.DealStatusLost,
		LostReason: &reason,
	}

	for name, semantic := range map[string]string{
		"re-decided as won": "won",
		"reopened":          "open",
	} {
		t.Run(name, func(t *testing.T) {
			store := &Store{clock: func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }}
			patch, status, err := store.stageTransitionPatch(
				context.Background(), nil, lost, AdvanceDealInput{}, semantic)
			if err != nil {
				t.Fatalf("stageTransitionPatch: %v", err)
			}
			if status == string(crmcontracts.DealStatusLost) {
				t.Fatalf("status = %q, want a landing that is not lost", status)
			}

			cleared, assigned := patch.After()["lost_reason"]
			if !assigned {
				t.Fatal("lost_reason was never assigned, so the previous reason survives the transition")
			}
			if cleared != nil {
				t.Errorf("lost_reason = %v, want nil — the reason describes a close that no longer stands", cleared)
			}
		})
	}
}

// An ordinary move between two open stages has no lost reason to clear, and
// must not say that it cleared one: a patch records every assignment it is
// given, so an unconditional clear would name lost_reason in the UPDATE and in
// the audit diff of every advance a deal ever makes.
func TestAnAdvanceWithNoLostReasonLeavesTheColumnOutOfThePatch(t *testing.T) {
	open := crmcontracts.Deal{Status: crmcontracts.DealStatusOpen}

	store := &Store{clock: func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }}
	patch, _, err := store.stageTransitionPatch(
		context.Background(), nil, open, AdvanceDealInput{}, "open")
	if err != nil {
		t.Fatalf("stageTransitionPatch: %v", err)
	}
	if _, assigned := patch.After()["lost_reason"]; assigned {
		t.Error("lost_reason was assigned on an advance that never touched it")
	}
}

// The reason still has to be written when the deal actually lands on lost,
// which is the behaviour the clearing branch must not swallow.
func TestALostReasonIsWrittenWhenTheDealLandsOnLost(t *testing.T) {
	reason := "budget cut"
	open := crmcontracts.Deal{Status: crmcontracts.DealStatusOpen}

	store := &Store{clock: func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }}
	patch, status, err := store.stageTransitionPatch(
		context.Background(), nil, open, AdvanceDealInput{LostReason: &reason}, "lost")
	if err != nil {
		t.Fatalf("stageTransitionPatch: %v", err)
	}
	if status != string(crmcontracts.DealStatusLost) {
		t.Fatalf("status = %q, want lost", status)
	}
	if got := patch.After()["lost_reason"]; got != reason {
		t.Errorf("lost_reason = %v, want %q", got, reason)
	}
}

func TestALostReasonIsTrimmedBeforeItIsStored(t *testing.T) {
	store := &Store{clock: func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }}
	open := crmcontracts.Deal{Status: crmcontracts.DealStatusOpen}
	padded := "  too expensive \n"
	patch, _, err := store.stageTransitionPatch(context.Background(), nil, open, AdvanceDealInput{LostReason: &padded}, "lost")
	if err != nil {
		t.Fatalf("stageTransitionPatch: %v", err)
	}
	if got := patch.After()["lost_reason"]; got != "too expensive" {
		t.Errorf("stored reason = %q, want it trimmed", got)
	}
	blank := " \t "
	if _, _, err := store.stageTransitionPatch(context.Background(), nil, open, AdvanceDealInput{LostReason: &blank}, "lost"); err == nil {
		t.Error("a blank reason closed the deal as lost")
	}
}
