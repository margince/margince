// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

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
	if got, ok := patch.After()["lost_reason"].(*string); !ok || got == nil || *got != reason {
		t.Errorf("lost_reason = %v, want %q", patch.After()["lost_reason"], reason)
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
	if got, ok := patch.After()["lost_reason"].(*string); !ok || got == nil || *got != "too expensive" {
		t.Errorf("stored reason = %v, want it trimmed", patch.After()["lost_reason"])
	}
	blank := " \t "
	if _, _, err := store.stageTransitionPatch(context.Background(), nil, open, AdvanceDealInput{LostReason: &blank}, "lost"); err == nil {
		t.Error("a blank reason closed the deal as lost")
	}
}

// A code names why a deal was lost. A move that loses nothing is refused one,
// so no code sits where no report reads it.
func TestALossCodeOnAnOpenMoveIsRefused(t *testing.T) {
	store := &Store{clock: func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }}
	code := "price"
	_, _, err := store.stageTransitionPatch(context.Background(), nil,
		crmcontracts.Deal{Status: crmcontracts.DealStatusOpen}, AdvanceDealInput{LostReasonCode: &code}, "open")
	if _, ok := errors.AsType[*LossReasonNotLostError](err); !ok {
		t.Fatalf("a code on an open move answered %v, want LossReasonNotLostError", err)
	}
}
