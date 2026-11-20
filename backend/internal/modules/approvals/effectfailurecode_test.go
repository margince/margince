// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"errors"
	"testing"
)

func TestEveryRecordedFailureSentenceCarriesACode(t *testing.T) {
	sentences := []string{lapsedRedemptionSentence, windowNotWidenedSentence}
	for _, err := range []error{ErrEffectFollowUpFailed, ErrEffectUnconfirmed, errors.New("executor down")} {
		sentences = append(sentences, effectFailureSentence(err))
	}
	seen := map[string]bool{}
	for _, sentence := range sentences {
		code := effectFailureCode(&sentence)
		if code == nil {
			t.Fatalf("no code for %q", sentence)
		}
		if seen[*code] {
			t.Fatalf("code %q names two sentences", *code)
		}
		seen[*code] = true
	}
	lapse := lapsedRedemptionSentence
	if got := effectFailureCode(&lapse); got == nil || *got != "approval_lapsed" {
		t.Fatalf("lapse code = %v, want approval_lapsed", got)
	}
	older := "a sentence in older words"
	if effectFailureCode(&older) != nil || effectFailureCode(nil) != nil {
		t.Fatal("an unknown or absent sentence must carry no code")
	}
}
