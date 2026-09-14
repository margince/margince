// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Addresses on the UPDATE request, which is how a bounced one gets fixed.
//
// The store has replaced a contact's addresses since the field existed and the
// CSV import path used it, but `UpdateContactRequest` never declared it — so the
// contract's own remedy for a dead address was to visit the contact page.
//
// Decoded from JSON rather than built as a struct: what is under test is that
// the field survives the wire into the store input, and a hand-built request
// would skip the half that was missing.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestContactUpdateInputCarriesTheAddressesItIsGiven(t *testing.T) {
	var req crmcontracts.UpdateContactRequest
	decodeInto(t, `{"emails":[
		{"email":"ada@new.example","email_type":"work","is_primary":true,"position":0},
		{"email":"ada@home.example","email_type":"personal"}
	]}`, &req)

	in := contactUpdateInput(req, nil)

	if len(in.Emails) != 2 {
		t.Fatalf("the patch carried %d addresses into the store, want the 2 sent",
			len(in.Emails))
	}
	if in.Emails[0].Email != "ada@new.example" || !in.Emails[0].IsPrimary {
		t.Errorf("the corrected address arrived as %+v", in.Emails[0])
	}
	if in.Emails[1].EmailType != "personal" {
		t.Errorf("the second address is typed %q, want personal", in.Emails[1].EmailType)
	}
	// Defaulted from the index, so a writer who omits position gets the order
	// they wrote rather than every address claiming zero.
	if in.Emails[1].Position != 1 {
		t.Errorf("an address sent without a position landed at %d, want its own index",
			in.Emails[1].Position)
	}
}

// Absent and empty are DIFFERENT answers, and the store reads the difference:
// nil leaves the addresses alone, an empty set removes them. A mapping that
// collapsed the two would make "this contact has no working address" unsayable.
func TestAnAbsentAddressListIsNotAnEmptyOne(t *testing.T) {
	var absent crmcontracts.UpdateContactRequest
	decodeInto(t, `{"full_name":"Ada Lovelace"}`, &absent)
	if got := contactUpdateInput(absent, nil).Emails; got != nil {
		t.Errorf("a patch that said nothing about addresses carried %v", got)
	}

	var cleared crmcontracts.UpdateContactRequest
	decodeInto(t, `{"emails":[]}`, &cleared)
	if got := contactUpdateInput(cleared, nil).Emails; got == nil {
		t.Error("a patch clearing every address carried nil, which the store reads " +
			"as 'leave them alone' — the removal would silently do nothing")
	}
}

// Create and update mean the same thing by an address, because one function
// maps both. Two loops would be two answers to what a default is.
func TestCreateAndUpdateMapAnAddressTheSameWay(t *testing.T) {
	const body = `{"emails":[{"email":"ada@new.example","email_type":"personal"}]}`

	var create crmcontracts.CreateContactRequest
	decodeInto(t, `{"full_name":"Ada Lovelace","source":"ui",`+body[1:], &create)
	made, err := contactCreateInput(create)
	if err != nil {
		t.Fatalf("mapping the create: %v", err)
	}

	var update crmcontracts.UpdateContactRequest
	decodeInto(t, body, &update)
	patched := contactUpdateInput(update, nil)

	if len(made.Emails) != 1 || len(patched.Emails) != 1 {
		t.Fatalf("create carried %d and update %d", len(made.Emails), len(patched.Emails))
	}
	if made.Emails[0] != patched.Emails[0] {
		t.Errorf("create maps the same address as %+v and update as %+v",
			made.Emails[0], patched.Emails[0])
	}
}

// An address added through the patch carries an origin.
//
// `source` is required on every ContactEmail the response returns, and
// UpdateContactRequest has no field to carry one — so without a default here the
// row lands with blank provenance and the response breaks its own contract.
//
// The integration tests next door cannot catch this: they call the store
// directly and pass `Source: "test"` themselves, which is the request mapper's
// job and exactly the step they skip.
func TestAnAddressAddedByPatchCarriesItsOrigin(t *testing.T) {
	var req crmcontracts.UpdateContactRequest
	decodeInto(t, `{"emails":[{"email":"ada@new.example"}]}`, &req)

	if got := contactUpdateInput(req, nil).Source; got != "manual" {
		t.Fatalf("the patch stamps its addresses %q, want the word this schema "+
			"uses for a contact doing something", got)
	}
}
