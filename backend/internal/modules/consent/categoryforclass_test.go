// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "testing"

// blockedReasonCode must read a suppression block's OWN kind rather than
// fall to the default arm meant for BlockObjection — the same mislabelling
// this file's own doc comment already exists to prevent for every other
// block code.
func TestBlockedReasonCodeNamesTheSuppressionThatBound(t *testing.T) {
	got := blockedReasonCode(Verdict{Code: BlockSuppressed, Suppression: "hard_bounce"})
	if got != "hard_bounce" {
		t.Errorf("blockedReasonCode(BlockSuppressed) = %q, want the verdict's own Suppression kind", got)
	}
}

// categoryForClass and resolutionForClass are two spellings of the same
// mapping, read from two different call sites (VerdictForContact's new
// suppression check and the transmit path's own category resolution). A
// caller that trusts the coarser one — VerdictForContact has to, since it
// answers before any message is resolved — relies on the two never
// disagreeing; this is the test that would fail the day they do.
func TestCategoryForClassAgreesWithResolutionForClass(t *testing.T) {
	for _, class := range []Class{
		ClassBusinessCorrespondence, ClassTransactional, ClassMarketing, ClassPhoneOutreach,
	} {
		got := categoryForClass(class)
		want := resolutionForClass(class).Category
		if got != want {
			t.Errorf("categoryForClass(%s) = %s, resolutionForClass(%s).Category = %s — VerdictForContact's suppression check would bind a different category than the transmit path resolves", class, got, class, want)
		}
	}
}
