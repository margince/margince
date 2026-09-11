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
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

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

// The same swap the address side has: pressing the primary radio on the other
// of two work numbers moves the primary between two RETAINED rows, so both
// travel the re-placement loop, and promoting one while the other is still
// primary is two live work primaries for the length of a statement unless the
// loop demotes before it promotes.
func TestSwappingWhichWorkPhoneIsPrimary(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Ada Lovelace",
		Phones: []PersonPhoneInput{
			{Phone: "+493011111111", PhoneType: "work", IsPrimary: false, Position: 0},
			{Phone: "+493022222222", PhoneType: "work", IsPrimary: true, Position: 1},
		},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	updated, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		Phones: []PersonPhoneInput{
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
// conflict uq_person_phone_primary would answer with.
func TestTwoPrimaryPhonesOfOneTypeIsRefusedByType(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Ada Lovelace",
		Phones:   []PersonPhoneInput{{Phone: "+493011111111", PhoneType: "work", IsPrimary: true, Position: 0}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	_, err = e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		Phones: []PersonPhoneInput{
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

// #4675: a person legitimately holds one number twice under two types. A PATCH
// that re-sends both rows unchanged — an ordinary rename that replaces the
// phone set every save — must not collapse them onto the last type.
func TestReplacingLeavesTwoTypesOfOneNumberIntact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Dup Phone",
		Phones: []PersonPhoneInput{
			{Phone: "+491119999999", PhoneType: "work", IsPrimary: false, Position: 0},
			{Phone: "+491119999999", PhoneType: "home", IsPrimary: false, Position: 1},
		},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	updated, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		Phones: []PersonPhoneInput{
			{Phone: "+491119999999", PhoneType: "work", IsPrimary: false, Position: 0},
			{Phone: "+491119999999", PhoneType: "home", IsPrimary: false, Position: 1},
		},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("re-sending both rows: %v", err)
	}

	rows := livePhoneRows(updated)
	types := map[string]bool{}
	for _, r := range rows {
		types[string(r.PhoneType)] = true
	}
	if len(rows) != 2 || !types["work"] || !types["home"] {
		t.Fatalf("phones = %+v, want one work and one home", rows)
	}
}

// Dropping one of two types of a number keeps the SURVIVING row's own identity.
// The by-value reconciler relabelled the FIRST held row of the number to the
// surviving type and archived the real one, so the survivor inherited the dropped
// row's id, created_at, source and captured_by — and observed_at, which decides
// whether a later provider fill stands, so a home row wearing a work row's older
// observed_at could be overwritten by a fill that should have been skipped. Any
// row pointing at the archived id then named a number that was still live.
func TestDroppingOneTypeKeepsTheSurvivorsIdentity(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Dup Phone",
		Phones: []PersonPhoneInput{
			{Phone: "+493011112222", PhoneType: "work", IsPrimary: false, Position: 0},
			{Phone: "+493011112222", PhoneType: "home", IsPrimary: false, Position: 1},
		},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	homeID := phoneIDOfType(t, person, "home")

	updated, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		// The reader kept only the home row.
		Phones: []PersonPhoneInput{{Phone: "+493011112222", PhoneType: "home", IsPrimary: false, Position: 0}},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("dropping the work row: %v", err)
	}

	rows := livePhoneRows(updated)
	if len(rows) != 1 || rows[0].PhoneType != "home" {
		t.Fatalf("phones = %+v, want a single home row", rows)
	}
	if rows[0].Id != homeID {
		t.Fatalf("survivor id = %v, want the HOME row's id %v — the work row was relabelled instead of the home row kept",
			rows[0].Id, homeID)
	}
}

func livePhoneRows(p crmcontracts.Person) []crmcontracts.PersonPhone {
	if p.Phones == nil {
		return nil
	}
	return *p.Phones
}

// phoneIDOfType reads the id of the one live row of the given type at create time.
func phoneIDOfType(t *testing.T, p crmcontracts.Person, phoneType string) openapi_types.UUID {
	t.Helper()
	for _, r := range livePhoneRows(p) {
		if string(r.PhoneType) == phoneType {
			return r.Id
		}
	}
	t.Fatalf("no %s phone in %+v", phoneType, p.Phones)
	return openapi_types.UUID{}
}
