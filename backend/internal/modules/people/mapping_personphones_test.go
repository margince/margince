// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Numbers on the UPDATE request, which is how a reassigned one gets fixed.
//
// `phones` was a person field on create and no field at all on update, so a
// phone correction returned 200 with the value discarded and no signal to the
// caller — the failure mode agents/recordfields_test.go names by hand, and the
// one `emails` was fixed for first.
//
// Decoded from JSON rather than built as a struct: what is under test is that
// the field survives the wire into the store input, and a hand-built request
// would skip the half that was missing.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestPersonUpdateInputCarriesTheNumbersItIsGiven(t *testing.T) {
	var req crmcontracts.UpdatePersonRequest
	decodeInto(t, `{"phones":[
		{"phone":"+49 30 111111","phone_type":"work","is_primary":true,"position":0},
		{"phone":"+49 170 222222","phone_type":"mobile"}
	]}`, &req)

	in := personUpdateInput(req, nil)

	if len(in.Phones) != 2 {
		t.Fatalf("the patch carried %d numbers into the store, want the 2 sent",
			len(in.Phones))
	}
	if in.Phones[0].Phone != "+49 30 111111" || !in.Phones[0].IsPrimary {
		t.Errorf("the corrected number arrived as %+v", in.Phones[0])
	}
	if in.Phones[1].PhoneType != "mobile" {
		t.Errorf("the second number is typed %q, want mobile", in.Phones[1].PhoneType)
	}
	// Defaulted from the index, so a writer who omits position gets the order
	// they wrote rather than every number claiming zero.
	if in.Phones[1].Position != 1 {
		t.Errorf("a number sent without a position landed at %d, want its own index",
			in.Phones[1].Position)
	}
}

// Absent and empty are DIFFERENT answers, the same way they are for addresses:
// nil leaves the numbers alone, an empty set removes them.
func TestAnAbsentPhoneListIsNotAnEmptyOne(t *testing.T) {
	var absent crmcontracts.UpdatePersonRequest
	decodeInto(t, `{"full_name":"Ada Lovelace"}`, &absent)
	if got := personUpdateInput(absent, nil).Phones; got != nil {
		t.Errorf("a patch that said nothing about numbers carried %v", got)
	}

	var cleared crmcontracts.UpdatePersonRequest
	decodeInto(t, `{"phones":[]}`, &cleared)
	if got := personUpdateInput(cleared, nil).Phones; got == nil {
		t.Error("a patch clearing every number carried nil, which the store reads " +
			"as 'leave them alone' — the removal would silently do nothing")
	}
}

// Create and update mean the same thing by a number, because one function maps
// both. Two loops would be two answers to what a default is.
func TestCreateAndUpdateMapANumberTheSameWay(t *testing.T) {
	const body = `{"phones":[{"phone":"+49 170 222222","phone_type":"mobile"}]}`

	var create crmcontracts.CreatePersonRequest
	decodeInto(t, `{"full_name":"Ada Lovelace","source":"ui",`+body[1:], &create)
	made, err := personCreateInput(create)
	if err != nil {
		t.Fatalf("mapping the create: %v", err)
	}

	var update crmcontracts.UpdatePersonRequest
	decodeInto(t, body, &update)
	patched := personUpdateInput(update, nil)

	if len(made.Phones) != 1 || len(patched.Phones) != 1 {
		t.Fatalf("create carried %d and update %d", len(made.Phones), len(patched.Phones))
	}
	if made.Phones[0] != patched.Phones[0] {
		t.Errorf("create maps the same number as %+v and update as %+v",
			made.Phones[0], patched.Phones[0])
	}
}
