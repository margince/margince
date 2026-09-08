// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Quick-find over a real Postgres, on the arm a name index cannot answer.
//
// A rep who pastes an address into the contact search was told "no contacts
// match these filters" for somebody plainly in the CRM: the query read the
// name tsvector and a trigram over the display name, and an address lives in
// person_email one table away. Same for a company and its domain.
//
// These run against the database rather than a rendered predicate because the
// arm is a subquery over a second table under row scope, and a clause that
// reads correctly can still return the wrong rows.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedContact writes one person with one address, both live.
func (e *privacyEnv) seedContact(t *testing.T, name, email string) ids.PersonID {
	t.Helper()
	id := ids.New[ids.PersonKind]()
	ctx := e.as(e.owner, principal.RowScopeOwn)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO person (id, full_name, owner_id, source, captured_by, visibility)
			VALUES ($1, $2, $3, 'manual', 'human:test', 'workspace')`, id, name, e.owner); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO person_email (person_id, email, email_type, is_primary, source, captured_by)
			VALUES ($1, $2, 'work', true, 'manual', 'human:test')`, id, email)
		return err
	}); err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return id
}

// seedAccount writes one company with one domain.
func (e *privacyEnv) seedAccount(t *testing.T, name, domain string) ids.CompanyID {
	t.Helper()
	id := ids.New[ids.CompanyKind]()
	ctx := e.as(e.owner, principal.RowScopeOwn)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO company (id, display_name, owner_id, source, captured_by)
			VALUES ($1, $2, $3, 'manual', 'human:test')`, id, name, e.owner); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
			VALUES ($1, $2, true, 'manual', 'human:test')`, id, domain)
		return err
	}); err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return id
}

func (e *privacyEnv) findPeople(t *testing.T, q string) []ids.UUID {
	t.Helper()
	rows, _, err := e.store.ListPeople(e.as(e.owner, principal.RowScopeAll), ListPeopleInput{Query: &q})
	if err != nil {
		t.Fatalf("searching people for %q: %v", q, err)
	}
	out := make([]ids.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, ids.UUID(r.Id))
	}
	return out
}

func (e *privacyEnv) findAccounts(t *testing.T, q string) []ids.UUID {
	t.Helper()
	rows, _, err := e.store.ListCompanies(e.as(e.owner, principal.RowScopeAll), ListCompaniesInput{Query: &q})
	if err != nil {
		t.Fatalf("searching companies for %q: %v", q, err)
	}
	out := make([]ids.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, ids.UUID(r.Id))
	}
	return out
}

func holds(list []ids.UUID, want ids.UUID) bool {
	for _, got := range list {
		if got == want {
			return true
		}
	}
	return false
}

func TestAContactIsFoundByTheAddressARepPastedIn(t *testing.T) {
	e := setupCapturePrivacy(t)
	judith := e.seedContact(t, "Judith Andresen", "judith@judithandresen.example")
	other := e.seedContact(t, "Someone Else", "someone@elsewhere.example")

	// The name still works, or the arm below proves nothing about the search
	// having gained anything.
	if got := e.findPeople(t, "Judith"); !holds(got, judith.UUID) {
		t.Fatal("the name search stopped finding her, so the address case says nothing")
	}
	for _, q := range []string{
		"judith@judithandresen.example",
		"JUDITH@JudithAndresen.Example",
		"  judith@judithandresen.example  ",
	} {
		got := e.findPeople(t, q)
		if !holds(got, judith.UUID) {
			t.Errorf("%q found %d contact(s), none of them hers — this is the search a rep "+
				"runs after copying a sender out of their mail client", q, len(got))
		}
		if holds(got, other.UUID) {
			t.Errorf("%q also returned an unrelated contact", q)
		}
	}
}

func TestAnAccountIsFoundByItsDomainAndByAnAddressAtIt(t *testing.T) {
	e := setupCapturePrivacy(t)
	account := e.seedAccount(t, "BERATUNG JUDITH ANDRESEN", "judithandresen.example")
	other := e.seedAccount(t, "Somebody Else GmbH", "elsewhere.example")

	if got := e.findAccounts(t, "BERATUNG"); !holds(got, account.UUID) {
		t.Fatal("the name search stopped finding the account")
	}
	for _, q := range []string{
		"judithandresen.example",
		// Pasting a person's address is how a rep most often has the domain.
		"judith@judithandresen.example",
	} {
		got := e.findAccounts(t, q)
		if !holds(got, account.UUID) {
			t.Errorf("%q found %d account(s), none of them theirs", q, len(got))
		}
		if holds(got, other.UUID) {
			t.Errorf("%q also returned an unrelated account", q)
		}
	}
}

func TestAnArchivedAddressDoesNotSurfaceItsContact(t *testing.T) {
	e := setupCapturePrivacy(t)
	person := e.seedContact(t, "Moved On", "old@former.example")
	ctx := e.as(e.owner, principal.RowScopeAll)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE person_email SET archived_at = now() WHERE person_id = $1`, person)
		return err
	}); err != nil {
		t.Fatalf("archiving the address: %v", err)
	}
	if got := e.findPeople(t, "old@former.example"); holds(got, person.UUID) {
		t.Error("an archived address still finds its contact — the arm reads rows the record no longer claims")
	}
	if got := e.findPeople(t, "Moved On"); !holds(got, person.UUID) {
		t.Error("archiving one address took the whole contact out of search")
	}
}

func TestTheIdentifierArmDoesNotWidenRowScope(t *testing.T) {
	e := setupCapturePrivacy(t)
	// A captured contact is the owner's alone until somebody promotes it.
	hidden := e.capturePerson(t, "owner")
	ctx := e.as(e.owner, principal.RowScopeOwn)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO person_email (person_id, email, email_type, is_primary, source, captured_by)
			VALUES ($1, 'private@captured.example', 'work', true, 'gmail:seed', 'connector:gmail')`, hidden)
		return err
	}); err != nil {
		t.Fatalf("seeding the captured contact's address: %v", err)
	}

	// The owner finds their own by address — the positive control, without
	// which a build that returned nothing for everybody would pass below.
	if got := e.findPeople(t, "private@captured.example"); !holds(got, hidden.UUID) {
		t.Fatal("the owner cannot find their own captured contact by address")
	}

	teammate := e.as(e.teammate, principal.RowScopeTeam)
	q := "private@captured.example"
	rows, _, err := e.store.ListPeople(teammate, ListPeopleInput{Query: &q})
	if err != nil {
		t.Fatalf("the teammate's search: %v", err)
	}
	for _, r := range rows {
		if ids.UUID(r.Id) == hidden.UUID {
			t.Error("a teammate found a captured contact by pasting its address — the identifier " +
				"arm answered about a row the record scope withholds")
		}
	}
}
