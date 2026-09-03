// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "testing"

// The conjunction is the rule a reader is most likely to "simplify" later, so
// it is pinned: one refused recipient refuses the message.
func TestOneDeniedRecipientRefusesTheWholeMessage(t *testing.T) {
	set := DecisionSet{Decisions: []Decision{
		{Verdict: VerdictAllow},
		{Verdict: VerdictReview, ReasonCode: ReasonNoEvidence},
		{Verdict: VerdictAllow},
	}}
	if set.Allowed() {
		t.Error("a set holding a review must not be allowed — a partial send is a lie about what happened")
	}
	if got := len(set.Denied()); got != 1 {
		t.Errorf("Denied() = %d, want the 1 recipient a message would have to name", got)
	}
}

// An empty set is not a permissive one. A message with no recipients has not
// been authorized; it has failed to ask.
func TestAnEmptySetIsNotAllowed(t *testing.T) {
	if (DecisionSet{}).Allowed() {
		t.Error("an empty decision set must not read as allowed")
	}
}

// The whole point of observe mode is that it does not block. The whole point of
// this table is the four things that block anyway.
func TestAbsoluteDenialsSurviveEveryMode(t *testing.T) {
	for _, reason := range []string{
		ReasonObjection, ReasonRestricted, ReasonHardBounce, ReasonUnconfirmedDOI,
		ReasonNoSubject, ReasonConsentWithdrawn,
	} {
		set := DecisionSet{Decisions: []Decision{{Verdict: VerdictDeny, ReasonCode: reason}}}
		for _, mode := range []Mode{ModeObserve, ModeWarn, ModeEnforce} {
			// legacyAllowed true is the hard case: the OLD gate would send.
			if set.Effective(mode, true) {
				t.Errorf("%s in %s mode: sent anyway — a rollout mode must not reach past this", reason, mode)
			}
		}
	}
}

// And the converse, or the test above would pass with everything denied.
func TestAnOrdinaryDenialDefersToTheModeWhileObserving(t *testing.T) {
	set := DecisionSet{Decisions: []Decision{{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence}}}
	if !set.Effective(ModeObserve, true) {
		t.Error("observe mode must let the old gate rule on a non-absolute disagreement")
	}
	if set.Effective(ModeEnforce, true) {
		t.Error("enforce mode must apply the engine's own refusal")
	}
}

// Enforce narrows, never widens: an engine allow cannot rescue a send the old
// gate refused while both still run.
func TestEnforceNeverOutranksTheOldGate(t *testing.T) {
	set := DecisionSet{Decisions: []Decision{{Verdict: VerdictAllow, ReasonCode: ReasonAllowed}}}
	if set.Effective(ModeEnforce, false) {
		t.Error("an engine allow must not send what the legacy gate refused")
	}
}

// The five service categories are the ones a hard suppression may not silence.
// Pinned both ways: a marketing send is emphatically not one of them.
func TestOnlySubjectServingCategoriesPassASuppression(t *testing.T) {
	serving := map[Category]bool{
		CategorySecurityNotice: true, CategoryPrivacyNotice: true,
		CategoryOptoutConfirmation: true, CategoryConsentConfirmation: true,
		CategoryRecordConfirmation: true,
	}
	for _, c := range Categories() {
		if got := c.ServesTheSubject(); got != serving[c] {
			t.Errorf("%s.ServesTheSubject() = %v, want %v", c, got, serving[c])
		}
	}
}

// There is no generic operational category. That absence IS the fix: naming a
// message "transactional" was how anything could call itself exempt.
func TestNoGenericTransactionalCategoryExists(t *testing.T) {
	for _, banned := range []Category{"transactional", "business_correspondence", "operational", "other"} {
		if banned.Valid() {
			t.Errorf("%q is a valid category — the escape hatch is back", banned)
		}
	}
	if len(Categories()) != 14 {
		t.Errorf("Categories() has %d members, want the 14 the vocabulary declares", len(Categories()))
	}
}
