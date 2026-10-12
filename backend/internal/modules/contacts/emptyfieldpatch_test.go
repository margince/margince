// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A connector's captured name of spaces is no name, and the fold treats it as
// absent: refusing it would drop the title and company the same capture carries.
func TestACapturedNameOfSpacesIsAbsentAndTheRestOfTheFoldStands(t *testing.T) {
	patch := emptyFieldPatch(crmcontracts.Lead{}, CapturedLeadFields{FullName: "   ", Title: "CTO"})
	if patch.FullName != nil {
		t.Errorf("a name of spaces was folded in as %q", *patch.FullName)
	}
	if patch.Title == nil || *patch.Title != "CTO" {
		t.Errorf("the title the same capture carried was lost: %v", patch.Title)
	}
}
