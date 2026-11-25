// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
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

// A captured name past the name limit could not be stored, so the fold leaves
// it out and still fills what else the capture carries.
func TestACapturedNamePastTheLimitIsLeftOutAndTheRestOfTheFoldStands(t *testing.T) {
	long := strings.Repeat("n", ContactNameLimit+1)
	patch := emptyFieldPatch(crmcontracts.Lead{}, CapturedLeadFields{FullName: long, Title: "CTO"})
	if patch.FullName != nil {
		t.Errorf("a %d-character name was folded in", len(long))
	}
	if patch.Title == nil || *patch.Title != "CTO" {
		t.Errorf("the title the same capture carried was lost: %v", patch.Title)
	}
	fits := strings.Repeat("n", ContactNameLimit)
	if patch := emptyFieldPatch(crmcontracts.Lead{}, CapturedLeadFields{FullName: fits}); patch.FullName == nil {
		t.Error("a name at the limit was left out")
	}
}
