// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A company page list that row scope shortens says how many entries it
// withheld and which colleagues own them, and nothing a withheld entry holds.
// Contacts are read by every seat unless capture-private, so each hidden
// contact here is made capture-private to its owner.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// hiddenReaderPerms is a team-scoped rep, so the scope clause is rendered: an
// unbounded caller skips it and would pass against a read with no clause.
var hiddenReaderPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":      {Read: true},
		"company":      {Read: true},
		"relationship": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

// seedDepartedSeat is a colleague who left: their records still exist and
// still count as withheld, and nobody is told to ask them.
func seedDepartedSeat(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := OwnerConn(t).Exec(context.Background(), `
		INSERT INTO app_user (id, email, display_name, email_domain_folded, status)
		VALUES ($1, 'departed@authz.test', 'Departed Colleague', 'authz.test', 'deactivated')`, id); err != nil {
		t.Fatalf("seeding a departed seat: %v", err)
	}
	return id
}

// seedEmployee makes a contact owned by owner a current employee of company.
// A private one is capture-private to owner, so other seats may not see it.
func seedEmployee(t *testing.T, e *Env, company ids.UUID, name string, owner ids.UUID, private bool) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, name, &owner)
	if private {
		e.MakeCapturePrivate(t, "contact", contact, owner)
	}
	contactID, companyID := ContactIDOf(contact), companyIDOf(company)
	primary := true
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, IsCurrentPrimary: &primary,
	}); err != nil {
		t.Fatalf("employing %s: %v", name, err)
	}
	return contact
}

func TestCompanyContactsStateHowManyAreHiddenAndWhoHoldsThem(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	seedEmployee(t, e, company, "Visible Buyer", e.Rep1, false)
	seedEmployee(t, e, company, "Hidden Sponsor", e.Rep3, true)
	seedEmployee(t, e, company, "Hidden Engineer", e.Rep3, true)
	seedEmployee(t, e, company, "Orphaned Champion", seedDepartedSeat(t), true)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, hiddenReaderPerms)
	page, err := companySurfaceService(e).Assemble(rep, companyIDOf(company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if page.Contacts == nil || len(page.Contacts.Data) != 1 {
		t.Fatalf("contacts = %+v, want only the one the rep may see", page.Contacts)
	}
	hidden := page.ContactsHidden
	if hidden == nil {
		t.Fatal("contacts_hidden is absent beside a listed contacts section")
	}
	if hidden.Count != 3 {
		t.Errorf("contacts_hidden.count = %d, want 3: two of Rep3's and the departed seat's", hidden.Count)
	}
	if len(hidden.Holders) != 1 || ids.UUID(hidden.Holders[0].UserId) != e.Rep3 {
		t.Errorf("holders = %+v, want Rep3 once and never the departed seat", hidden.Holders)
	}
	wire, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, withheld := range []string{"Hidden Sponsor", "Hidden Engineer", "Orphaned Champion"} {
		if strings.Contains(string(wire), withheld) {
			t.Errorf("the page carries the withheld contact name %q", withheld)
		}
	}
}

func TestAnUnboundedReaderIsToldNothingIsHidden(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	seedEmployee(t, e, company, "Rep3's Contact", e.Rep3, true)

	page, err := companySurfaceService(e).Assemble(e.Admin(), companyIDOf(company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if page.ContactsHidden == nil || page.ContactsHidden.Count != 0 || len(page.ContactsHidden.Holders) != 0 {
		t.Errorf("contacts_hidden = %+v, want count 0 and no holders for an unbounded reader", page.ContactsHidden)
	}
}

func TestHiddenBillingContactsNameTheirOwnerNotThemselves(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	mine := e.SeedContact(t, "Visible Payer", &e.Rep1)
	theirs := e.SeedContact(t, "Hidden Payer", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", theirs, e.Rep3)
	seedBillingEdge(t, mine, company, "recipient")
	seedBillingEdge(t, theirs, company, "approver")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, billingReaderPerms)
	page, err := companySurfaceService(e).Assemble(rep, companyIDOf(company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	hidden := page.BillingContactsHidden
	if hidden == nil || hidden.Count != 1 || len(hidden.Holders) != 1 || ids.UUID(hidden.Holders[0].UserId) != e.Rep3 {
		t.Errorf("billing_contacts_hidden = %+v, want one entry held by Rep3", hidden)
	}
}
