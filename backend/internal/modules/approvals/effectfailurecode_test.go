// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"errors"
	"slices"
	"testing"
)

func TestEachFailureCodeStoresItsPinnedSentence(t *testing.T) {
	pinned := []effectFailure{
		{"approval_lapsed", "Approval expired before execution. Nothing changed. Request a new proposal to try again."},
		{"effect_not_run", "this was approved, but the work it released did not run"},
		{"effect_unconfirmed", "this was approved and sent on, but the system it went to never confirmed it, " +
			"so it may have happened. Check the record before you ask for it again"},
		{"follow_up_failed", "this was approved and the change was made, but the automation that asked for it " +
			"could not be marked finished"},
		{"window_not_widened", "the agent's window could not be widened, so the approval has not taken effect"},
	}
	if !slices.Equal(effectFailures, pinned) {
		t.Fatalf("failures = %q, want %q: a client words each code, so a code and its sentence change together", effectFailures, pinned)
	}
	for _, failure := range pinned {
		stored := failure.sentence
		if code := effectFailureCode(&stored); code == nil || *code != failure.code {
			t.Errorf("stored %q reads back as %v, want %s", stored, code, failure.code)
		}
	}
	unknown := "a sentence no failure stores"
	if effectFailureCode(&unknown) != nil || effectFailureCode(nil) != nil {
		t.Error("an unknown or absent sentence must carry no code")
	}
}

func TestAFailedEffectIsToldByItsCause(t *testing.T) {
	for err, want := range map[error]effectFailure{
		ErrEffectFollowUpFailed:     failureFollowUpFailed,
		ErrEffectUnconfirmed:        failureUnconfirmed,
		errors.New("executor down"): failureNotRun,
	} {
		if got := effectFailureOf(err); got != want {
			t.Errorf("effectFailureOf(%v) = %s, want %s", err, got.code, want.code)
		}
	}
}
