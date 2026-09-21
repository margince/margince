// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The precondition an import undo attaches to every reversal, asked of the
// verbs that carry it.
//
// The undo reads which of a page's rows a human has touched and then archives
// the untouched ones. Those were two transactions, so somebody editing a row in
// the window between them had their edit reversed anyway — the check said
// untouched, and by the time the archive ran it was not. The window is small,
// which is the only reason it was tolerable; it is not a reason it is right.
//
// What settles it is a precondition the WRITE re-asks under its own row lock,
// and that is what these hold: each verb refuses when a human audit row exists
// after the instant the caller named, and behaves exactly as before when no
// caller names one.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnArchiveAskedOnlyForAnUntouchedRecordRefusesATouchedOne(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	company := e.SeedCompany(t, "Imported Ltd", nil)
	companyID := ids.From[ids.CompanyKind](company)
	contact := e.SeedContact(t, "Imported Contact", nil)
	contactID := ids.From[ids.ContactKind](contact)

	// The instant the import landed the row, taken AFTER the seed and never
	// before it: the undo's own reference is import_record_map.created_at, and
	// the creation of the row is not somebody touching it. Read before the
	// seed, this instant makes the row's own creation audit a human edit — and
	// then the untouched case passes or fails on how fast the fixture ran.
	imported := seedInstant(t)

	// A human acts on both — any human-actor audit row counts, which is the
	// point: somebody who independently archived an imported row is exactly
	// who this protects, and a narrower filter would reverse their act too.
	name := "Imported Ltd (renamed by a contact)"
	if _, err := e.Contacts.UpdateCompany(admin, companyID, contacts.UpdateCompanyInput{
		DisplayName: &name,
	}); err != nil {
		t.Fatalf("the human edit: %v", err)
	}
	title := "Head of Something"
	if _, err := e.Contacts.UpdateContact(admin, contactID, contacts.UpdateContactInput{Title: &title}); err != nil {
		t.Fatalf("the human edit: %v", err)
	}

	var touched *contacts.HumanTouchedError
	if _, err := e.Contacts.ArchiveCompany(admin, companyID, nil,
		contacts.NotTouchedByHumanSince(imported)); !errors.As(err, &touched) {
		t.Errorf("archiving a company a contact edited → %v, want the touched refusal — an undo "+
			"reversing it would take away the edit they made", err)
	}
	if _, err := e.Contacts.ArchiveContact(admin, contactID, nil,
		contacts.NotTouchedByHumanSince(imported)); !errors.As(err, &touched) {
		t.Errorf("archiving a contact a contact edited → %v, want the touched refusal", err)
	}

	// The record survives the refusal: a precondition that failed must leave
	// the row exactly as the contact left it.
	after, err := e.Contacts.GetCompany(admin, companyID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the company back: %v", err)
	}
	if after.ArchivedAt != nil {
		t.Error("the company was archived anyway")
	}
}

// And the same verbs, with no precondition and with one nobody has broken,
// behave exactly as they did. A guard that refused either would have stopped
// every other caller of these three verbs.
func TestAnArchiveWithNoPreconditionOrAnUnbrokenOneStillLands(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	untouched := e.SeedCompany(t, "Untouched Ltd", nil)
	imported := seedInstant(t)
	if _, err := e.Contacts.ArchiveCompany(admin, ids.From[ids.CompanyKind](untouched), nil,
		contacts.NotTouchedByHumanSince(imported)); err != nil {
		t.Errorf("archiving a company nobody touched → %v, want it to land", err)
	}

	plain := e.SeedCompany(t, "Plain Ltd", nil)
	if _, err := e.Contacts.ArchiveCompany(admin, ids.From[ids.CompanyKind](plain), nil); err != nil {
		t.Errorf("archiving with no precondition at all → %v, want the behaviour every other caller "+
			"of this verb has always had", err)
	}
}

// seedInstant is now, read from the DATABASE's clock rather than the test
// process's.
//
// The comparison this fixture sets up is against audit_log.occurred_at, which
// Postgres writes with its own now(). A Go instant is a second clock, and the
// two agree only as closely as the machines do — which on a CI runner is close
// enough to pass most of the time and not close enough to be a test.
func seedInstant(t *testing.T) time.Time {
	t.Helper()
	var at time.Time
	if err := OwnerConn(t).QueryRow(t.Context(), `SELECT now()`).Scan(&at); err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	return at
}
