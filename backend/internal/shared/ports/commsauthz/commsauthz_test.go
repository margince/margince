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

// everywhere lifts one mode into the per-category resolver Effective takes, for
// a test whose subject is the mode itself rather than which category carries
// it. A test about two categories at once builds its own resolver instead.
func everywhere(m Mode) func(Category) Mode {
	return func(Category) Mode { return m }
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
			if set.Effective(everywhere(mode), true) {
				t.Errorf("%s in %s mode: sent anyway — a rollout mode must not reach past this", reason, mode)
			}
		}
	}
}

// And the converse, or the test above would pass with everything denied.
func TestAnOrdinaryDenialDefersToTheModeWhileObserving(t *testing.T) {
	set := DecisionSet{Decisions: []Decision{{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence}}}
	if !set.Effective(everywhere(ModeObserve), true) {
		t.Error("observe mode must let the old gate rule on a non-absolute disagreement")
	}
	if set.Effective(everywhere(ModeEnforce), true) {
		t.Error("enforce mode must apply the engine's own refusal")
	}
}

// UNDER ENFORCE THE ENGINE ALONE DECIDES, and this is the case that says so:
// an engine allow sends a message the old purpose gate refused.
//
// That is the point of enforcing rather than an accident of it. The old gate
// answers on a caller-supplied purpose key and its business-correspondence arm
// reads qualifying events only, so an ordinary reply to a thread the subject
// started is refused for want of a consent row nobody had reason to record.
// The engine resolves that reply from the thread itself. Keeping the
// conjunction would leave the weaker authority able to overrule the stronger
// one, which is the regression this rollout exists to end.
//
// The test previously asserted the opposite — correctly, while every category
// still observed and both authorities ran together.
func TestUnderEnforceTheEngineAloneDecides(t *testing.T) {
	set := DecisionSet{Decisions: []Decision{{Verdict: VerdictAllow, ReasonCode: ReasonAllowed}}}
	if !set.Effective(everywhere(ModeEnforce), false) {
		t.Error("an engine allow was overruled by the old gate under enforce")
	}
	// And an engine denial still refuses, or the line above would be a bypass
	// rather than an authority.
	denied := DecisionSet{Decisions: []Decision{{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence}}}
	if denied.Effective(everywhere(ModeEnforce), true) {
		t.Error("an engine denial was overruled by the old gate under enforce")
	}
}

// A CATEGORY STILL OBSERVING DEFERS, which is what keeps enforce a per-category
// rollout rather than a switch. A set with no enforced recipient takes the old
// gate's answer in both directions.
func TestAnObservedCategoryStillDefersToTheOldGate(t *testing.T) {
	set := DecisionSet{Decisions: []Decision{{Verdict: VerdictAllow, ReasonCode: ReasonAllowed}}}
	if set.Effective(everywhere(ModeObserve), false) {
		t.Error("an observed category sent what the old gate refused")
	}
	if !set.Effective(everywhere(ModeObserve), true) {
		t.Error("an observed category refused what the old gate allowed")
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

// The mode is read per recipient, not once for the message. One send can carry
// two categories — a reply to a thread that copies somebody the engine calls
// marketing — and a single mode for the set would have to pick one of them,
// which means either enforcing a category the installation has not enforced or
// observing one it has.
func TestEachRecipientIsJudgedUnderItsOwnCategorysMode(t *testing.T) {
	set := DecisionSet{Decisions: []Decision{
		{Verdict: VerdictAllow, ReasonCode: ReasonAllowed, Resolved: CategoryReplyToInbound},
		{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence, Resolved: CategoryMarketing},
	}}
	// Replies enforce; marketing still observes, which is the state this
	// product ships in until the jurisdiction packs land.
	repliesOnly := func(c Category) Mode {
		if c == CategoryReplyToInbound {
			return ModeEnforce
		}
		return ModeObserve
	}
	if !set.Effective(repliesOnly, true) {
		t.Error("a marketing refusal blocked the send while marketing was still observing")
	}
	// And the converse: enforce marketing and the same set refuses.
	marketingToo := func(Category) Mode { return ModeEnforce }
	if set.Effective(marketingToo, true) {
		t.Error("marketing was enforced and its refusal did not bind")
	}
}

// An empty set is not an allow. It means the engine was asked about nobody,
// and a message with no authorized recipient is not one that may go out.
func TestAnEmptySetPermitsNothing(t *testing.T) {
	if (DecisionSet{}).Effective(everywhere(ModeObserve), true) {
		t.Error("a set with no decisions permitted a send")
	}
}
