// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"maps"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A case decision may prefer only a member it merges.
func TestACasePreferenceMustNameASelectedMember(t *testing.T) {
	t.Parallel()
	a, b := ids.NewV7(), ids.NewV7()
	d := ContactCaseDecision{
		MemberIDs: []ids.UUID{a, b}, SurvivorID: a, OwnerID: ids.NewV7(),
		Prefer: MergePreference{MergeFieldEmail: b},
	}
	if _, err := caseSelection(d); err != nil {
		t.Fatalf("a preference for a selected member was refused: %v", err)
	}
	d.Prefer = MergePreference{MergeFieldEmail: ids.NewV7()}
	var refusal *DedupeInputError
	if _, err := caseSelection(d); !errors.As(err, &refusal) || refusal.Field != "prefer.email" {
		t.Errorf("a preference for an outsider answered %v, want a refusal naming prefer.email", err)
	}
}

func TestTheContractPreferenceKeepsEveryNamedSlot(t *testing.T) {
	t.Parallel()
	member := ids.NewV7()
	in := &crmcontracts.ContactCasePreference{Name: new(openapi_types.UUID(member)), Employment: new(openapi_types.UUID(member))}
	want := MergePreference{MergeFieldName: member, MergeFieldEmployment: member}
	if got := MergePreferenceOf(in); !maps.Equal(got, want) {
		t.Errorf("MergePreferenceOf = %v, want %v", got, want)
	}
	if got := MergePreferenceOf(nil); got != nil {
		t.Errorf("no preference read as %v, want none", got)
	}
}

// Rule 2 for the scalar slots: a preferred source replaces, any other fills.
func TestTheSurvivorshipPatchFollowsThePreference(t *testing.T) {
	t.Parallel()
	source := ids.From[ids.ContactKind](ids.NewV7())
	target := crmcontracts.Contact{FullName: "Ann Lee", FirstName: new("Ann"), LastName: new("Lee"), Title: new("Rep")}
	merged := crmcontracts.Contact{FullName: "Annie Lee-Park", FirstName: new("Annie"), LastName: new("Lee-Park"), Title: new("Head")}

	if after := buildSurvivorshipPatch(target, merged, source, nil).After(); len(after) != 0 {
		t.Errorf("with no preference the survivor's values changed: %v", after)
	}
	after := buildSurvivorshipPatch(target, merged, source, MergePreference{MergeFieldName: source.UUID, MergeFieldTitle: source.UUID}).After()
	want := map[string]any{contactNameColumn: "Annie Lee-Park", columnFirstName: "Annie", columnLastName: "Lee-Park", "title": "Head"}
	if !maps.Equal(after, want) {
		t.Errorf("with the source preferred the patch = %v, want %v", after, want)
	}
	survivorNamed := MergePreference{MergeFieldName: ids.NewV7()}
	unnamed := crmcontracts.Contact{FullName: "Ann Lee"}
	if after := buildSurvivorshipPatch(unnamed, merged, source, survivorNamed).After(); len(after) != 1 {
		t.Errorf("a name another member won took this member's parts: %v", after)
	}
	untitled := target
	untitled.Title = nil
	if after := buildSurvivorshipPatch(untitled, merged, source, nil).After(); after["title"] != "Head" {
		t.Errorf("an empty title was not filled: %v", after)
	}
}
