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

// A captured name, company and title past their columns' limit fold in cut.
// An approved fold never fails on text nobody can shorten.
func TestACapturedTextPastItsLimitFoldsInCut(t *testing.T) {
	long := strings.Repeat("n", ContactNameLimit+1)
	patch := emptyFieldPatch(crmcontracts.Lead{}, CapturedLeadFields{FullName: long, CompanyName: long, Title: long})
	for field, got := range map[string]*string{"full_name": patch.FullName, "company_name": patch.CompanyName, "title": patch.Title} {
		if got == nil || *got != long[:ContactNameLimit] {
			t.Errorf("%s folded in as %v, want the first %d characters", field, got, ContactNameLimit)
		}
	}
}
