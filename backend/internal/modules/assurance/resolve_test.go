// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"strings"
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

// An answer that hides a finding says why. The next contact to see the number is
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
	// that contacts stop giving them.
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

// The system's own answer is not a contact's to give. Claiming it would say the
// condition stopped being true without anything having checked.
func TestAContactCannotClaimTheConditionCleared(t *testing.T) {
	t.Parallel()
	now := when()

	if _, err := checkResolution(Resolution{Outcome: OutcomeConditionCleared}, now); err == nil {
		t.Error("a contact claimed condition_cleared — that is recorded by the check itself")
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

// A deferral hides a finding as a suppressing answer does. It stays out of the
// bundle and out of tasks until it comes back, so it takes the same ceiling.
// Without one, deferring for two years hides a finding for two years.
func TestADeferralIsCappedAtNinetyDays(t *testing.T) {
	t.Parallel()
	now := when()

	tooFar := now.Add(2 * 365 * 24 * time.Hour)
	if _, err := checkResolution(Resolution{
		Outcome: OutcomeRemindLater, RemindAt: &tooFar,
	}, now); err == nil {
		t.Error("a two-year deferral was accepted, so the finding is hidden for two years")
	}

	// The ceiling itself stands, so a caller may defer for the longest the
	// answer allows without guessing at an epsilon.
	atCeiling := now.Add(MaxSuppression)
	if _, err := checkResolution(Resolution{
		Outcome: OutcomeRemindLater, RemindAt: &atCeiling,
	}, now); err != nil {
		t.Errorf("a deferral to the ceiling itself was refused: %v", err)
	}

	inside := now.Add(14 * 24 * time.Hour)
	if _, err := checkResolution(Resolution{
		Outcome: OutcomeRemindLater, RemindAt: &inside,
	}, now); err != nil {
		t.Errorf("a two-week deferral was refused: %v", err)
	}
}

// The readers cap what was already written, because answers given before the
// refusal above still name a moment past the ceiling.
func TestTheLiveDeferralReadsTheCeilingFromWhenItWasGiven(t *testing.T) {
	t.Parallel()
	sql := LiveRemindAtSQL("r", "$7")

	for _, want := range []string{"r.remind_at", "r.created_at", "$7", "LEAST("} {
		if !strings.Contains(sql, want) {
			t.Errorf("the live deferral reads %q, which names no %s", sql, want)
		}
	}
	// Measured from when the answer was given, not from now: an answer given in
	// May may hide its finding until August, whenever it is read.
	if strings.Contains(sql, "now()") {
		t.Errorf("the live deferral reads %q, which measures the ceiling from the read", sql)
	}
	if days := MaxSuppressionDays(); days != 90 {
		t.Errorf("the ceiling is %d days, want the 90 MaxSuppression names", days)
	}
}
