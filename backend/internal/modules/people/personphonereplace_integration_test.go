// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Replacing a person's phone numbers, against the real index.
//
// This lives at the integration tier for the reason its address twin does: the
// promise is one only Postgres can answer.
//
//	uq_person_phone_primary (person_id, phone_type) WHERE is_primary AND archived_at IS NULL
//
// Per phone_type, NOT one primary per person — so a corrected work number walks
// straight into the slot the stored one still holds. There is deliberately no
// dedupe index across people here, unlike addresses: a switchboard reaches
// several people, and that is not a duplicate.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The ordinary correction: somebody's work number was reassigned.
//
// It meets the primary slot head-on. The person holds a primary work number,
// the patch names a different one, and both are primary work rows for as long
// as the write leaves them both live — which the index refuses. Archiving
// before inserting is what makes that window not exist.
func TestReplacingTheWorkPhoneCorrectsIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Ada Lovelace",
		Phones:   []PersonPhoneInput{{Phone: "+493011111111", PhoneType: "work", IsPrimary: true, Position: 1}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	updated, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		Phones: []PersonPhoneInput{{Phone: "+493022222222", PhoneType: "work", IsPrimary: true, Position: 1}},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("replacing the work number: %v", err)
	}
	rows := livePhoneRows(updated)
	if len(rows) != 1 || rows[0].Phone != "+493022222222" {
		t.Fatalf("phones = %+v, want only the corrected number", rows)
	}
}

// A patch that says nothing about numbers leaves them standing. This is the
// half a nil-vs-empty mistake breaks silently: every patch of an unrelated
// field would strip the record's numbers.
func TestAPatchThatNamesNoNumberLeavesThemAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Ada Lovelace",
		Phones:   []PersonPhoneInput{{Phone: "+493011111111", PhoneType: "work", IsPrimary: true}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	name := "Ada Byron"
	updated, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		FullName: &name,
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("renaming: %v", err)
	}
	if rows := livePhoneRows(updated); len(rows) != 1 {
		t.Errorf("phones = %+v after a rename, want the stored number untouched", rows)
	}
}

// An empty list is a real answer and removes them all — a contact who no longer
// has a number anybody can reach is a fact worth recording.
func TestAnEmptyPhoneListRemovesThem(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Ada Lovelace",
		Phones:   []PersonPhoneInput{{Phone: "+493011111111", PhoneType: "work", IsPrimary: true}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	updated, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		Phones: []PersonPhoneInput{},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("clearing the numbers: %v", err)
	}
	if rows := livePhoneRows(updated); len(rows) != 0 {
		t.Errorf("phones = %+v after an empty list, want none", rows)
	}
}

func livePhoneRows(p crmcontracts.Person) []crmcontracts.PersonPhone {
	if p.Phones == nil {
		return nil
	}
	return *p.Phones
}
