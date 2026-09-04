// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Replacing a person's email addresses, against the real indexes.
//
// These live at the integration tier because every promise here is one only
// Postgres can answer. Two partial unique indexes decide all of it:
//
//	uq_person_email_dedupe   (email) WHERE archived_at IS NULL
//	uq_person_email_primary  (person_id, email_type) WHERE is_primary AND archived_at IS NULL
//
// The second one is per email_type, NOT one primary per person, and a
// spreadsheet correcting somebody's work address walks straight into both.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The ordinary correction: someone's work address changed and the file says so.
//
// It meets the primary slot head-on. The person holds a primary work address,
// the file names a different one, and both are primary work rows for as long as
// the write leaves them both live — which uq_person_email_primary refuses.
// Archiving before inserting is what makes that window not exist.
func TestReplacingTheWorkEmailCorrectsIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Ada Lovelace",
		Emails:   []PersonEmailInput{{Email: "ada@old.example", EmailType: "work", IsPrimary: true, Position: 1}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	updated, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		Emails: []PersonEmailInput{{Email: "ada@new.example", EmailType: "work", IsPrimary: true, Position: 1}},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("replacing the work address: %v", err)
	}
	rows := liveEmailRows(updated)
	if len(rows) != 1 || string(rows[0].Email) != "ada@new.example" {
		t.Fatalf("emails = %+v, want only the corrected address", rows)
	}
}

// The file names one address; the person holds two of different types. The
// address the file never mentioned survives, and it keeps its OWN primary flag —
// the slot is per email_type, so demoting every retained row would silently
// strip a person's primary personal address for naming a work one.
func TestReplacingOneAddressKeepsTheOtherTypesPrimary(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Grace Hopper",
		Emails: []PersonEmailInput{
			{Email: "grace@work.example", EmailType: "work", IsPrimary: true, Position: 1},
			{Email: "grace@home.example", EmailType: "personal", IsPrimary: true, Position: 2},
		},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// What an import produces: the file's one work address, plus every stored
	// address it did not name, carried through unchanged.
	updated, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		Emails: []PersonEmailInput{
			{Email: "grace@new.example", EmailType: "work", IsPrimary: true, Position: 1},
			{Email: "grace@home.example", EmailType: "personal", IsPrimary: true, Position: 2},
		},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("correcting the work address: %v", err)
	}
	rows := liveEmailRows(updated)
	if len(rows) != 2 {
		t.Fatalf("emails = %+v, want the corrected work address and the untouched personal one", rows)
	}
	primaries := map[string]string{}
	for _, row := range rows {
		if row.IsPrimary {
			primaries[string(row.EmailType)] = string(row.Email)
		}
	}
	if primaries["work"] != "grace@new.example" || primaries["personal"] != "grace@home.example" {
		t.Fatalf("primaries = %v, want one per type and the personal one untouched", primaries)
	}
}

// An address another person holds is refused, and the target person's own rows
// are left exactly as they were: the transaction rolls back whole.
func TestReplacingWithAnAddressAnotherPersonHoldsIsRefused(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	if _, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Katherine Johnson",
		Emails:   []PersonEmailInput{{Email: "katherine@nasa.example", EmailType: "work", IsPrimary: true, Position: 1}},
		Source:   "test",
	}); err != nil {
		t.Fatalf("create incumbent: %v", err)
	}
	other, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Mary Jackson",
		Emails:   []PersonEmailInput{{Email: "mary@nasa.example", EmailType: "work", IsPrimary: true, Position: 1}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	_, err = e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(other.Id)), UpdatePersonInput{
		Emails: []PersonEmailInput{{Email: "katherine@nasa.example", EmailType: "work", IsPrimary: true, Position: 1}},
		Source: "test",
	})
	var dup *DuplicateEmailError
	if !errors.As(err, &dup) {
		t.Fatalf("replace with a claimed address → %v, want DuplicateEmailError", err)
	}

	after, err := e.store.GetPerson(ctx, ids.From[ids.PersonKind](ids.UUID(other.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("re-reading the person: %v", err)
	}
	rows := liveEmailRows(after)
	if len(rows) != 1 || string(rows[0].Email) != "mary@nasa.example" {
		t.Fatalf("emails = %+v, want the person's own address untouched", rows)
	}
}

// Re-stating the address a person already holds is not refused by that person's
// own row — the claim check excludes them, which is why the self-excluding
// variant exists at all.
func TestReStatingAHeldAddressIsNotAConflict(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Annie Easley",
		Emails:   []PersonEmailInput{{Email: "annie@nasa.example", EmailType: "work", IsPrimary: true, Position: 1}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		Emails: []PersonEmailInput{{Email: "annie@nasa.example", EmailType: "work", IsPrimary: true, Position: 1}},
		Source: "test",
	}); err != nil {
		t.Fatalf("re-stating a held address: %v", err)
	}
}

// A replaced address is ARCHIVED, never deleted. person_email is the row that
// makes an address name one person, and the evidence a merge dispute is settled
// by is that the person once held it.
func TestAReplacedAddressIsArchivedNotDeleted(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Dorothy Vaughan",
		Emails:   []PersonEmailInput{{Email: "dorothy@old.example", EmailType: "work", IsPrimary: true, Position: 1}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		Emails: []PersonEmailInput{{Email: "dorothy@new.example", EmailType: "work", IsPrimary: true, Position: 1}},
		Source: "test",
	}); err != nil {
		t.Fatalf("replacing the address: %v", err)
	}

	var archived int
	if err := e.store.db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM person_email WHERE person_id = $1 AND archived_at IS NOT NULL`,
		ids.UUID(person.Id)).Scan(&archived); err != nil {
		t.Fatalf("counting archived rows: %v", err)
	}
	if archived != 1 {
		t.Fatalf("archived rows = %d, want the replaced address kept as history", archived)
	}
}

// liveEmailRows reads the person's address rows, which the wire type carries as
// an optional slice.
func liveEmailRows(p crmcontracts.Person) []crmcontracts.PersonEmail {
	if p.Emails == nil {
		return nil
	}
	return *p.Emails
}

// The case a CSV import actually produces: emailsMergedOnto keeps the stored
// work address alongside the file's new one, so BOTH are work rows and only one
// may be primary. The retained row arrives demoted, and the write must land the
// new primary without ever having two live primaries of one type.
func TestANewPrimaryLandsBesideARetainedWorkAddress(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Melba Roy",
		Emails:   []PersonEmailInput{{Email: "melba@old.example", EmailType: "work", IsPrimary: true, Position: 1}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	if _, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), UpdatePersonInput{
		Emails: []PersonEmailInput{
			{Email: "melba@new.example", EmailType: "work", IsPrimary: true, Position: 1},
			{Email: "melba@old.example", EmailType: "work", IsPrimary: false, Position: 2},
		},
		Source: "test",
	}); err != nil {
		t.Fatalf("landing a new primary beside the retained address: %v", err)
	}

	after, err := e.store.GetPerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	rows := liveEmailRows(after)
	if len(rows) != 2 {
		t.Fatalf("emails = %+v, want both addresses live", rows)
	}
	var primary string
	for _, row := range rows {
		if row.IsPrimary {
			primary = string(row.Email)
		}
	}
	if primary != "melba@new.example" {
		t.Fatalf("primary = %q, want the file's address", primary)
	}
}

// Sending an empty set removes every address, which is a real answer.
//
// A contact who no longer has a working address is a fact worth recording, and
// it is the shape a bounce correction reaches when the last address is the dead
// one. Nil and empty are therefore DIFFERENT: nil leaves the addresses alone,
// and the mapping is careful to keep the two apart because the store reads that
// difference — `replacePersonEmails` returns early on nil.
//
// The removal rides `email <> ALL($2)` with an empty keep list, which is TRUE
// for every stored row. Asserted rather than reasoned about, because an
// implementation that treated empty as "nothing to do" would look identical
// from the call site and quietly leave the dead address in place.
func TestAnEmptyAddressSetRemovesEveryAddress(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Ada Lovelace",
		Emails: []PersonEmailInput{
			{Email: "ada@work.example", EmailType: "work", IsPrimary: true},
			{Email: "ada@home.example", EmailType: "personal"},
		},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if len(liveEmailRows(person)) != 2 {
		t.Fatalf("the fixture starts with %d addresses, want 2", len(liveEmailRows(person)))
	}

	updated, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)),
		UpdatePersonInput{Emails: []PersonEmailInput{}, Source: "test"})
	if err != nil {
		t.Fatalf("clearing the addresses: %v", err)
	}

	if rows := liveEmailRows(updated); len(rows) != 0 {
		t.Fatalf("the person still holds %d addresses after being cleared: %+v",
			len(rows), rows)
	}
}

// And an absent set leaves them alone, without which the case above could be a
// mapping that sends an empty slice for everything.
func TestAnAbsentAddressSetLeavesThemAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Ada Lovelace",
		Emails:   []PersonEmailInput{{Email: "ada@work.example", EmailType: "work", IsPrimary: true}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	title := "CTO"
	updated, err := e.store.UpdatePerson(ctx, ids.From[ids.PersonKind](ids.UUID(person.Id)),
		UpdatePersonInput{Title: &title, Source: "test"})
	if err != nil {
		t.Fatalf("patching the title: %v", err)
	}

	if rows := liveEmailRows(updated); len(rows) != 1 {
		t.Fatalf("a patch that said nothing about addresses left %d of them: %+v",
			len(rows), rows)
	}
}
