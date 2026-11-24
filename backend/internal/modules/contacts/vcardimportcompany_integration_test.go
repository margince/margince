// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// An import started from one company's Contacts tab. Each contact the import
// creates joins that company in the card's own transaction, and the company the
// card names is not used.

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedImportCompany creates the company an import is made for.
func seedImportCompany(t *testing.T, e *dedupeEnv, name string) ids.CompanyID {
	t.Helper()
	company, err := e.store.CreateCompany(e.as(), CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id))
}

func importCardsToCompany(t *testing.T, e *dedupeEnv, company ids.CompanyID, file string) ([]VCardResult, error) {
	t.Helper()
	entries, err := ParseVCards(strings.NewReader(file))
	if err != nil {
		t.Fatalf("parsing the file: %v", err)
	}
	return e.store.ImportVCardsToCompany(e.as(), entries, company)
}

func TestImportingToACompanyFilesEveryCreatedContactUnderItInsteadOfTheCardsOwnCompany(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	home := seedImportCompany(t, e, "Home Account GmbH")

	results, err := importCardsToCompany(t, e, home,
		"BEGIN:VCARD\nFN:Ada Atrium\nORG:Somewhere Else AG\nEMAIL:ada@atrium.example\nEND:VCARD\n"+
			"BEGIN:VCARD\nFN:Ben Bracket\nEMAIL:ben@bracket.example\nEND:VCARD\n")
	if err != nil {
		t.Fatalf("importing: %v", err)
	}

	if len(results) != 2 || results[0].Outcome != VCardCreated || results[1].Outcome != VCardCreated {
		t.Fatalf("outcomes = %+v, want two created", results)
	}
	for _, result := range results {
		assertVCardEmployment(ctx, t, e, *result.ContactID, "Home Account GmbH")
	}
	var strays int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM company WHERE display_name = 'Somewhere Else AG'`).Scan(&strays)
	}); err != nil {
		t.Fatalf("counting the card's own company: %v", err)
	}
	if strays != 0 {
		t.Errorf("the card's own company was created %d times beside the one the import was for", strays)
	}
}

func TestImportingToACompanyLeavesAMatchedContactsEmployerAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	home := seedImportCompany(t, e, "Home Account GmbH")
	first := importCards(ctx, t, e, "BEGIN:VCARD\nFN:Cora Cadence\nORG:Elsewhere Inc\nEMAIL:cora@cadence.example\nEND:VCARD\n")

	results, err := importCardsToCompany(t, e, home,
		"BEGIN:VCARD\nFN:Cora Cadence\nTITLE:Director\nEMAIL:cora@cadence.example\nEND:VCARD\n")
	if err != nil {
		t.Fatalf("importing: %v", err)
	}

	if len(results) != 1 || results[0].Outcome != VCardUpdated || *results[0].ContactID != *first[0].ContactID {
		t.Fatalf("outcomes = %+v, want the first contact updated", results)
	}
	assertVCardEmployment(ctx, t, e, *first[0].ContactID, "Elsewhere Inc")
}

func TestImportingToACompanyTheCallerCannotReadWritesNothingNotEvenTheUpdates(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	cora := importCards(ctx, t, e, "BEGIN:VCARD\nFN:Cora Cadence\nEMAIL:cora@cadence.example\nEND:VCARD\n")[0]

	_, err := importCardsToCompany(t, e, ids.From[ids.CompanyKind](ids.NewV7()),
		"BEGIN:VCARD\nFN:Cora Cadence\nTITLE:Director\nEMAIL:cora@cadence.example\nEND:VCARD\n"+
			"BEGIN:VCARD\nFN:Dina Dangling\nEMAIL:dina@dangling.example\nEND:VCARD\n")

	if err == nil {
		t.Fatal("an import into a company nobody has was accepted")
	}
	if contacts, audits, events := rowsNamed(t, e, "Dina Dangling"); contacts+audits+events != 0 {
		t.Errorf("a refused import left %d contacts, %d audit rows and %d events", contacts, audits, events)
	}
	var title string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT coalesce(title, '') FROM contact WHERE id = $1`, *cora.ContactID).Scan(&title)
	}); err != nil {
		t.Fatalf("reading the matched contact's title: %v", err)
	}
	if title != "" {
		t.Errorf("a refused import still wrote the title %q on a matched contact", title)
	}
}
