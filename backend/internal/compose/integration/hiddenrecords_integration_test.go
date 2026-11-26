// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A company page list that row scope shortens says how many entries it
// withheld and which colleagues own them, and nothing a withheld entry holds.
// Contacts are read by every seat unless capture-private, so each hidden
// contact here is made capture-private to its owner.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
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

// deactivateThroughTheDirectory makes a colleague leave through the product's
// own path, with its cascade, after the records they own were written. Their
// records still count as withheld, and nobody is told to ask them.
func deactivateThroughTheDirectory(t *testing.T, e *Env, seat ids.UUID) {
	t.Helper()
	admin := identity.Identity{
		UserID: ids.From[ids.UserKind](e.AdminUser), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS),
		Roles: []string{"admin"}, SeatType: string(principal.SeatFull), Permissions: AdminPerms,
	}
	if err := identity.NewService(e.Pool).DeactivateUser(e.Admin(), admin,
		identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](seat)}); err != nil {
		t.Fatalf("deactivating a seat: %v", err)
	}
}

// seedEmployee makes a contact owned by owner a current employee of company.
// A private one is capture-private to owner, so other seats may not see it.
func seedEmployee(t *testing.T, e *Env, company ids.UUID, name string, owner ids.UUID, private bool) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, name, &owner)
	contactID, companyID := ContactIDOf(contact), companyIDOf(company)
	primary := true
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, IsCurrentPrimary: &primary,
	}); err != nil {
		t.Fatalf("employing %s: %v", name, err)
	}
	// Private only once employed: a capture-private contact answers to its
	// owner alone, so the admin who writes the edge could no longer reach it.
	if private {
		e.MakeCapturePrivate(t, "contact", contact, owner)
	}
	return contact
}

func TestCompanyContactsStateHowManyAreHiddenAndWhoHoldsThem(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	seedEmployee(t, e, company, "Visible Buyer", e.Rep1, false)
	seedEmployee(t, e, company, "Hidden Sponsor", e.Rep3, true)
	seedEmployee(t, e, company, "Hidden Engineer", e.Rep3, true)
	seedEmployee(t, e, company, "Orphaned Champion", e.Rep2, true)
	deactivateThroughTheDirectory(t, e, e.Rep2)
	// Archived: counting it would show as a fourth entry.
	archived := seedEmployee(t, e, company, "Archived Lead", e.Rep3, true)
	e.WsExec(t, `UPDATE contact SET archived_at = now() WHERE id = $1`, archived)

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
		t.Errorf("contacts_hidden.count = %d, want 3: two of Rep3's and the departed Rep2's, never the archived one", hidden.Count)
	}
	if len(hidden.Holders) != 1 || ids.UUID(hidden.Holders[0].UserId) != e.Rep3 {
		t.Errorf("holders = %+v, want Rep3 once and never the departed Rep2", hidden.Holders)
	}
	wire, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, withheld := range []string{"Hidden Sponsor", "Hidden Engineer", "Orphaned Champion", "Archived Lead"} {
		if strings.Contains(string(wire), withheld) {
			t.Errorf("the page carries the withheld contact name %q", withheld)
		}
	}
}

// An admin reads every contact except a capture-private one, which answers to
// its owner alone. So the admin is told about that one, and only that one.
func TestAnAdminIsToldOnlyAboutTheCapturePrivateContacts(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	seedEmployee(t, e, company, "Rep3's Shared Contact", e.Rep3, false)
	seedEmployee(t, e, company, "Rep3's Private Contact", e.Rep3, true)

	page, err := companySurfaceService(e).Assemble(e.Admin(), companyIDOf(company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if page.Contacts == nil || len(page.Contacts.Data) != 1 {
		t.Fatalf("contacts = %+v, want the shared contact only", page.Contacts)
	}
	hidden := page.ContactsHidden
	if hidden == nil || hidden.Count != 1 || len(hidden.Holders) != 1 || ids.UUID(hidden.Holders[0].UserId) != e.Rep3 {
		t.Errorf("contacts_hidden = %+v, want the one private contact, held by Rep3", hidden)
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
