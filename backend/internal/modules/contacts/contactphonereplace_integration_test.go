// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Replacing a contact's phone numbers, against the real index.
//
// This lives at the integration tier for the reason its address twin does: the
// promise is one only Postgres can answer.
//
//	uq_contact_phone_primary (contact_id, phone_type) WHERE is_primary AND archived_at IS NULL
//
// Per phone_type, NOT one primary per contact — so a corrected work number walks
// straight into the slot the stored one still holds. There is deliberately no
// dedupe index across contacts here, unlike addresses: a switchboard reaches
// several contacts, and that is not a duplicate.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The ordinary correction: somebody's work number was reassigned.
//
// It meets the primary slot head-on. The contact holds a primary work number,
// the patch names a different one, and both are primary work rows for as long
// as the write leaves them both live — which the index refuses. Archiving
// before inserting is what makes that window not exist.
func TestReplacingTheWorkPhoneCorrectsIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Ada Lovelace",
		Phones:   []ContactPhoneInput{{Phone: "+493011111111", PhoneType: "work", IsPrimary: true, Position: 1}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	updated, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(contact.Id)), UpdateContactInput{
		Phones: []ContactPhoneInput{{Phone: "+493022222222", PhoneType: "work", IsPrimary: true, Position: 1}},
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

	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Ada Lovelace",
		Phones:   []ContactPhoneInput{{Phone: "+493011111111", PhoneType: "work", IsPrimary: true}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	name := "Ada Byron"
	updated, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(contact.Id)), UpdateContactInput{
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

	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Ada Lovelace",
		Phones:   []ContactPhoneInput{{Phone: "+493011111111", PhoneType: "work", IsPrimary: true}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	updated, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(contact.Id)), UpdateContactInput{
		Phones: []ContactPhoneInput{},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("clearing the numbers: %v", err)
	}
	if rows := livePhoneRows(updated); len(rows) != 0 {
		t.Errorf("phones = %+v after an empty list, want none", rows)
	}
}

// The same swap the address side has: pressing the primary radio on the other
// of two work numbers moves the primary between two RETAINED rows, so both
// travel the re-placement loop, and promoting one while the other is still
// primary is two live work primaries for the length of a statement unless the
// loop demotes before it promotes.
func TestSwappingWhichWorkPhoneIsPrimary(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Ada Lovelace",
		Phones: []ContactPhoneInput{
			{Phone: "+493011111111", PhoneType: "work", IsPrimary: false, Position: 0},
			{Phone: "+493022222222", PhoneType: "work", IsPrimary: true, Position: 1},
		},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	updated, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(contact.Id)), UpdateContactInput{
		Phones: []ContactPhoneInput{
			{Phone: "+493011111111", PhoneType: "work", IsPrimary: true, Position: 0},
			{Phone: "+493022222222", PhoneType: "work", IsPrimary: false, Position: 1},
		},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("swapping which work number is primary: %v", err)
	}
	rows := livePhoneRows(updated)
	if len(rows) != 2 {
		t.Fatalf("phones = %+v, want both numbers live", rows)
	}
	var primary string
	for _, row := range rows {
		if row.IsPrimary {
			primary = row.Phone
		}
	}
	if primary != "+493011111111" {
		t.Fatalf("primary = %q, want the number the swap promoted", primary)
	}
}

// The same contradiction on numbers: two primaries of one type is refused
// before any write, and the refusal names the type rather than the bare
// conflict uq_contact_phone_primary would answer with.
func TestTwoPrimaryPhonesOfOneTypeIsRefusedByType(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Ada Lovelace",
		Phones:   []ContactPhoneInput{{Phone: "+493011111111", PhoneType: "work", IsPrimary: true, Position: 0}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	_, err = e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(contact.Id)), UpdateContactInput{
		Phones: []ContactPhoneInput{
			{Phone: "+493011111111", PhoneType: "work", IsPrimary: true, Position: 0},
			{Phone: "+493022222222", PhoneType: "work", IsPrimary: true, Position: 1},
		},
		Source: "test",
	})
	var conflict *PrimaryConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("two work primaries → %v, want PrimaryConflictError", err)
	}
	if conflict.Type != "work" {
		t.Fatalf("conflict names type %q, want work", conflict.Type)
	}
}

func livePhoneRows(p crmcontracts.Contact) []crmcontracts.ContactPhone {
	if p.Phones == nil {
		return nil
	}
	return *p.Phones
}
