// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAllowedByOverrideFlipsToAllowAndNamesTheRow(t *testing.T) {
	id := ids.NewV7()
	d := Decision{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence}
	got := d.AllowedByOverride(id)
	if got.Verdict != VerdictAllow {
		t.Errorf("verdict = %q, want allow", got.Verdict)
	}
	if got.ReasonCode != ReasonAllowedByOverride {
		t.Errorf("reason = %q, want %q", got.ReasonCode, ReasonAllowedByOverride)
	}
	if got.OverrideID != id {
		t.Error("override id was not recorded on the decision")
	}
}

func TestAnOverrideAllowedDecisionIsNotItselfOverrulable(t *testing.T) {
	d := Decision{Verdict: VerdictAllow, ReasonCode: ReasonAllowedByOverride}
	if d.CanBeOverruled() {
		t.Error("an allow must never be overrulable")
	}
}

// TestUnknownPurposeIsOverrulableButNotByCategory pins the one refusal the two
// predicates must disagree on. An unknown_purpose deny is machine-level and
// non-absolute, so CanBeOverruled is true — but it resolved to NO category, so a
// per-category override has nothing to answer and CanBeOverruledByCategory is
// false. Left equal, a rep's marketing vouch would flip a send whose purpose the
// engine could not resolve.
func TestUnknownPurposeIsOverrulableButNotByCategory(t *testing.T) {
	unknown := Decision{Verdict: VerdictDeny, ReasonCode: ReasonUnknownPurpose}
	if !unknown.CanBeOverruled() {
		t.Error("unknown_purpose is machine-level and non-absolute; CanBeOverruled must stay true, " +
			"or this test's premise moved")
	}
	if unknown.CanBeOverruledByCategory() {
		t.Error("unknown_purpose resolved to no category; a per-category override must not answer it")
	}

	// A genuinely category-resolved refusal is answerable by both: it named the
	// one category a vouch can address.
	for _, reason := range []string{ReasonNoMarketingConsent, ReasonNoEvidence} {
		d := Decision{Verdict: VerdictDeny, ReasonCode: reason}
		if !d.CanBeOverruled() || !d.CanBeOverruledByCategory() {
			t.Errorf("%q resolves to a category; both predicates must be true", reason)
		}
	}

	// An allow has nothing to overrule, by either predicate.
	allowed := Decision{Verdict: VerdictAllow, ReasonCode: ReasonAllowed}
	if allowed.CanBeOverruled() || allowed.CanBeOverruledByCategory() {
		t.Error("an allow reports as overrulable")
	}
}

// TestOnlyUnknownPurposeIsExcludedByCategory pins the exclusion to exactly the
// no-category reason. Every OTHER machine-level, non-absolute refusal resolves
// to a category and stays answerable by a per-category override, so the two
// predicates agree on all of them — the day a future reason resolves to no
// category, it must be added to CanBeOverruledByCategory's exclusion and to this
// list together, rather than silently becoming overrulable by a vouch it does
// not name.
func TestOnlyUnknownPurposeIsExcludedByCategory(t *testing.T) {
	for _, reason := range []string{ReasonNoEvidence, ReasonNoMarketingConsent} {
		d := Decision{Verdict: VerdictDeny, ReasonCode: reason}
		if d.CanBeOverruled() != d.CanBeOverruledByCategory() {
			t.Errorf("%q: the predicates disagree, but only unknown_purpose resolves to no category",
				reason)
		}
	}
	unknown := Decision{Verdict: VerdictDeny, ReasonCode: ReasonUnknownPurpose}
	if unknown.CanBeOverruled() == unknown.CanBeOverruledByCategory() {
		t.Error("unknown_purpose is the one reason the predicates must part on")
	}
}
