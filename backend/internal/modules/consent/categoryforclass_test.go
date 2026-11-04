// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

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

// categoryForClass and resolutionForClass are two spellings of one mapping.
// The verdict's suppression check reads one, the transmit path's category
// resolution the other. The verdict trusts the coarser one,
// because it answers before any message is resolved. This test fails the day
// the two disagree.
func TestCategoryForClassAgreesWithResolutionForClass(t *testing.T) {
	for _, class := range []Class{
		ClassBusinessCorrespondence, ClassTransactional, ClassMarketing, ClassPhoneOutreach,
	} {
		got := categoryForClass(class)
		want := resolutionForClass(class).Category
		if got != want {
			t.Errorf("categoryForClass(%s) = %s, resolutionForClass(%s).Category = %s — the verdict's suppression check would bind a different category than the transmit path resolves", class, got, class, want)
		}
	}
}

// The classes privacy keeps a withdrawal for are the classes whose withdrawal
// stops marketing here, so the two never read a withdrawal differently.
func TestMarketingPurposeClassesAreTheMarketingClasses(t *testing.T) {
	got := commsauthz.MarketingPurposeClasses()
	want := []string{string(ClassMarketing), string(ClassPhoneOutreach)}
	if !slices.Equal(got, want) {
		t.Errorf("MarketingPurposeClasses() = %v, want %v", got, want)
	}
}
