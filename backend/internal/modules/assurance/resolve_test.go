// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"testing"
	"time"
)

// The day every case below is read on. The rules are about intervals from
// "now", so the date itself carries nothing.
func when() time.Time {
	return time.Date(testYear, time.May, 14, 12, 0, 0, 0, time.UTC)
}

// A suppressing answer hides a finding from the screens a revenue commitment is
// made from. The ceiling is what stops that being permanent.
func TestSuppressionIsCappedAtNinetyDays(t *testing.T) {
	t.Parallel()
	now := when()

	// Asked for a year. Refused rather than silently shortened: a caller who
	// asked for a year and got ninety days without being told would believe
	// the finding stays hidden through the next two quarters.
	tooLong := now.Add(365 * 24 * time.Hour)
	if _, err := checkResolution(Resolution{
		Outcome: OutcomeValueCorrect, Reason: "checked against the signed order",
		ExpiresAt: &tooLong,
	}, now); err == nil {
		t.Error("a year-long suppression was accepted — a value that was correct in May " +
			"is a claim about May")
	}

	// Inside the ceiling stands as asked.
	inside := now.Add(30 * 24 * time.Hour)
	got, err := checkResolution(Resolution{
		Outcome: OutcomeValueCorrect, Reason: "checked against the signed order",
		ExpiresAt: &inside,
	}, now)
	if err != nil {
		t.Fatalf("a thirty-day suppression was refused: %v", err)
	}
	if got == nil || !got.Equal(inside) {
		t.Errorf("the stored expiry is %v, want the caller's own %v", got, inside)
	}

	// Asked for nothing at all gets the ceiling, not forever.
	got, err = checkResolution(Resolution{
		Outcome: OutcomeValueCorrect, Reason: "checked against the signed order",
	}, now)
	if err != nil {
		t.Fatalf("an answer with no expiry was refused: %v", err)
	}
	if got == nil {
		t.Fatal("an answer with no expiry stored none — the suppression would be permanent")
	}
	if !got.Equal(now.Add(MaxSuppression)) {
		t.Errorf("the default expiry is %v, want the ceiling %v", got, now.Add(MaxSuppression))
	}
}

// An answer that hides a finding says why. The next person to see the number is
// owed the reason it is not flagged.
func TestASuppressingAnswerNamesItsReason(t *testing.T) {
	t.Parallel()
	now := when()

	for _, outcome := range []string{OutcomeValueCorrect, OutcomeNotRelevant} {
		if _, err := checkResolution(Resolution{Outcome: outcome}, now); err == nil {
			t.Errorf("%s was accepted with no reason", outcome)
		}
	}
	// The admitting half: an answer that does NOT hide anything needs no
	// reason, and requiring one would make the common answers tedious enough
	// that people stop giving them.
	for _, outcome := range []string{OutcomeFixedRecord, OutcomeAddedEvidence, OutcomeReassign} {
		if _, err := checkResolution(Resolution{Outcome: outcome}, now); err != nil {
			t.Errorf("%s was refused with no reason (%v) — it hides nothing", outcome, err)
		}
	}
}

// "Not now" is an answer about WHEN, not about whether.
func TestADeferralNamesWhenItComesBack(t *testing.T) {
	t.Parallel()
	now := when()

	if _, err := checkResolution(Resolution{Outcome: OutcomeRemindLater}, now); err == nil {
		t.Error("a deferral with no date was accepted — it is a dismissal wearing a " +
			"different word")
	}
	past := now.Add(-24 * time.Hour)
	if _, err := checkResolution(Resolution{Outcome: OutcomeRemindLater, RemindAt: &past}, now); err == nil {
		t.Error("a deferral to yesterday was accepted")
	}
	future := now.Add(7 * 24 * time.Hour)
	if _, err := checkResolution(Resolution{Outcome: OutcomeRemindLater, RemindAt: &future}, now); err != nil {
		t.Errorf("a deferral to next week was refused: %v", err)
	}
}

// The system's own answer is not a person's to give. Claiming it would say the
// condition stopped being true without anything having checked.
func TestAPersonCannotClaimTheConditionCleared(t *testing.T) {
	t.Parallel()
	now := when()

	if _, err := checkResolution(Resolution{Outcome: OutcomeConditionCleared}, now); err == nil {
		t.Error("a person claimed condition_cleared — that is recorded by the check itself")
	}
	if _, err := checkResolution(Resolution{Outcome: "made_it_up"}, now); err == nil {
		t.Error("an outcome outside the vocabulary was accepted")
	}
	// The admitting half. Without it a validator refusing EVERY outcome would
	// pass both assertions above.
	if _, err := checkResolution(Resolution{Outcome: OutcomeFixedRecord}, now); err != nil {
		t.Errorf("a real answer was refused: %v", err)
	}
}
