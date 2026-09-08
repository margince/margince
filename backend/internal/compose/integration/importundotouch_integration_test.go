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

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnArchiveAskedOnlyForAnUntouchedRecordRefusesATouchedOne(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	// The instant an import would have landed the row: everything the fixture
	// does afterwards is "since" it.
	imported := time.Now().UTC()

	org := e.SeedOrg(t, "Imported Ltd", nil)
	orgID := ids.From[ids.OrganizationKind](org)
	person := e.SeedPerson(t, "Imported Contact", nil)
	personID := ids.From[ids.PersonKind](person)

	// A human acts on both — any human-actor audit row counts, which is the
	// point: somebody who independently archived an imported row is exactly
	// who this protects, and a narrower filter would reverse their act too.
	name := "Imported Ltd (renamed by a person)"
	if _, err := e.People.UpdateOrganization(admin, orgID, people.UpdateOrganizationInput{
		DisplayName: &name,
	}); err != nil {
		t.Fatalf("the human edit: %v", err)
	}
	title := "Head of Something"
	if _, err := e.People.UpdatePerson(admin, personID, people.UpdatePersonInput{Title: &title}); err != nil {
		t.Fatalf("the human edit: %v", err)
	}

	var touched *people.HumanTouchedError
	if _, err := e.People.ArchiveOrganization(admin, orgID, nil,
		people.NotTouchedByHumanSince(imported)); !errors.As(err, &touched) {
		t.Errorf("archiving a company a person edited → %v, want the touched refusal — an undo "+
			"reversing it would take away the edit they made", err)
	}
	if _, err := e.People.ArchivePerson(admin, personID, nil,
		people.NotTouchedByHumanSince(imported)); !errors.As(err, &touched) {
		t.Errorf("archiving a contact a person edited → %v, want the touched refusal", err)
	}

	// The record survives the refusal: a precondition that failed must leave
	// the row exactly as the person left it.
	after, err := e.People.GetOrganization(admin, orgID, storekit.LiveOnly)
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
	imported := time.Now().UTC()

	untouched := e.SeedOrg(t, "Untouched Ltd", nil)
	if _, err := e.People.ArchiveOrganization(admin, ids.From[ids.OrganizationKind](untouched), nil,
		people.NotTouchedByHumanSince(imported)); err != nil {
		t.Errorf("archiving a company nobody touched → %v, want it to land", err)
	}

	plain := e.SeedOrg(t, "Plain Ltd", nil)
	if _, err := e.People.ArchiveOrganization(admin, ids.From[ids.OrganizationKind](plain), nil); err != nil {
		t.Errorf("archiving with no precondition at all → %v, want the behaviour every other caller "+
			"of this verb has always had", err)
	}
}
