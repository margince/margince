// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestALeadNameIsHeldToTheContactNameLimit(t *testing.T) {
	t.Parallel()
	if _, err := LeadCreationInput(crmcontracts.CreateLeadRequest{FullName: new(strings.Repeat("é", ContactNameLimit))}); err != nil {
		t.Errorf("a name at the limit: %v, want admitted", err)
	}
	_, err := LeadCreationInput(crmcontracts.CreateLeadRequest{FullName: new(strings.Repeat("é", ContactNameLimit+1))})
	assertNameTooLong(t, "create", err)

	_, _, err = buildLeadPatch(crmcontracts.Lead{FullName: new("Lee")}, UpdateLeadInput{FullName: new(strings.Repeat("n", ContactNameLimit+1))})
	assertNameTooLong(t, "edit", err)
}

func TestAStoredLeadNamePastTheLimitMayBeSentBackUnchanged(t *testing.T) {
	t.Parallel()
	stored := strings.Repeat("n", ContactNameLimit+5)
	if _, _, err := buildLeadPatch(crmcontracts.Lead{FullName: &stored}, UpdateLeadInput{FullName: new(stored)}); err != nil {
		t.Errorf("the stored name sent back: %v, want admitted", err)
	}
}

func TestACapturedLeadNamePastTheLimitLeavesTheBlank(t *testing.T) {
	t.Parallel()
	patch := emptyFieldPatch(crmcontracts.Lead{}, CapturedLeadFields{FullName: strings.Repeat("n", ContactNameLimit+1), Title: "CTO"})
	if patch.FullName != nil {
		t.Errorf("a captured name past the limit was patched in: %d characters", len(*patch.FullName))
	}
	if patch.Title == nil {
		t.Error("the other captured fields must still fill their blanks")
	}
}

func assertNameTooLong(t *testing.T, door string, err error) {
	t.Helper()
	tooLong, ok := errors.AsType[*ContactNameTooLongError](err)
	if !ok {
		t.Fatalf("%s: err = %v, want a ContactNameTooLongError", door, err)
	}
	if field, code, _ := tooLong.FieldFault(); field != contactNameColumn || code != "too_long" {
		t.Errorf("%s: fault = %s %s, want full_name too_long", door, field, code)
	}
}
