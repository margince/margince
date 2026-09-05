// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// Whether an answer stops a message, and whether anybody may lift it.
//
// Two questions a surface must not recombine for itself, kept beside the rules
// that answer them.

import "testing"

// TestWouldRefuseFollowsTheModeExceptWhereNothingMay holds the rule a composer
// draws its refusal from.
//
// The verdict alone is not the answer, and that is the whole reason this exists:
// a surface reading `deny` under observe would tell a rep a message cannot go
// when it is about to go, which talks them out of lawful mail.
func TestWouldRefuseFollowsTheModeExceptWhereNothingMay(t *testing.T) {
	t.Parallel()

	// A refusal the rollout position may soften: recorded under observe, acted
	// on under enforce.
	soft := Decision{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence}
	if soft.WouldRefuse(ModeObserve) {
		t.Error("an ordinary deny stops the message under observe; observe records, it does not block")
	}
	if !soft.WouldRefuse(ModeEnforce) {
		t.Error("an ordinary deny does not stop the message under enforce")
	}

	// A refusal no mode reaches past.
	hard := Decision{Verdict: VerdictDeny, ReasonCode: ReasonObjection}
	for _, mode := range []Mode{ModeObserve, ModeWarn, ModeEnforce} {
		if !hard.WouldRefuse(mode) {
			t.Errorf("an objection does not stop the message under %q; no rollout position "+
				"softens an absolute denial", mode)
		}
	}

	// An allow is never a refusal, whatever the mode.
	allowed := Decision{Verdict: VerdictAllow, ReasonCode: ReasonAllowed}
	for _, mode := range []Mode{ModeObserve, ModeWarn, ModeEnforce} {
		if allowed.WouldRefuse(mode) {
			t.Errorf("an allow reads as a refusal under %q", mode)
		}
	}
}

// TestOnlyANonAbsoluteMachineReadingCanBeOverruled is the rule that decides
// whether a surface offers a control at all.
//
// The two axes disagree on purpose, and this is where that matters: four
// reasons are the engine's own reading AND absolute. Offering an override there
// renders a button that cannot do what it promises — the rep types a
// justification and the staging gate refuses anyway.
func TestOnlyANonAbsoluteMachineReadingCanBeOverruled(t *testing.T) {
	t.Parallel()

	// The case the override exists for: the engine found nothing, and a rep who
	// watched the customer ask for the mail knows better.
	unproven := Decision{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence}
	if !unproven.CanBeOverruled() {
		t.Error("an unevidenced refusal cannot be overruled; that is the one case the override is for")
	}

	// Machine-level AND absolute: a fact to correct, not a decision to argue with.
	for _, reason := range []string{
		ReasonHardBounce, ReasonFrequencyCapReached,
		ReasonNoSubject, ReasonUnconfirmedDOI,
	} {
		d := Decision{Verdict: VerdictDeny, ReasonCode: reason}
		if LevelForReason(reason) != LevelMachine {
			t.Errorf("%q is no longer machine-level; this test's premise moved", reason)
		}
		if d.CanBeOverruled() {
			t.Errorf("%q offers an override, but it is absolute — a rep would type a "+
				"justification the staging gate then ignores", reason)
		}
	}

	// The subject's own act: nobody lifts it, and the surface must not suggest otherwise.
	for _, reason := range []string{ReasonObjection, ReasonRestricted, ReasonConsentWithdrawn} {
		d := Decision{Verdict: VerdictDeny, ReasonCode: reason}
		if d.CanBeOverruled() {
			t.Errorf("%q offers an override, but it is the subject's own act", reason)
		}
	}

	// An allow has nothing to overrule.
	if (Decision{Verdict: VerdictAllow, ReasonCode: ReasonAllowed}).CanBeOverruled() {
		t.Error("an allow reports as overrulable")
	}
}
