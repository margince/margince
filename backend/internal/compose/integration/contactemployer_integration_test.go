// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The employer every contact read carries: the contact list's company column and
// the record read answer it from one attach, so these drive both.

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// employContact records an employment edge through the writer production uses,
// ended when a date is given. A hand-inserted row would prove nothing about the
// rows the product makes — the current-primary flag among them.
func employContact(t *testing.T, e *Env, contact, company ids.UUID, ended *time.Time) {
	t.Helper()
	contactID := ids.From[ids.ContactKind](contact)
	companyID := ids.From[ids.CompanyKind](company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind:             "employment",
		ContactID:        &contactID,
		CompanyID:        &companyID,
		IsCurrentPrimary: boolPtr(ended == nil),
		EndedAt:          ended,
		Source:           "manual",
	}); err != nil {
		t.Fatalf("seeding the employment edge: %v", err)
	}
}

// listedContact is one row of the contact list, by id.
func listedContact(ctx context.Context, t *testing.T, e *Env, contact ids.UUID) crmcontracts.Contact {
	t.Helper()
	page, _, err := e.Contacts.ListContacts(ctx, contacts.ListContactsInput{})
	if err != nil {
		t.Fatalf("listing contacts: %v", err)
	}
	for _, row := range page {
		if ids.UUID(row.Id) == contact {
			return row
		}
	}
	t.Fatalf("the contact list returned %d rows, none of them the contact under test", len(page))
	return crmcontracts.Contact{}
}

// Today's employer, on the list row and on the record read alike — the two
// surfaces share one attach, so a reader asking "who is this and where do they
// work" gets the same answer wherever they ask.
func TestAContactNamesTheEmployerTheyHoldToday(t *testing.T) {
	e := Setup(t)
	acme := e.SeedCompany(t, "Acme", nil)
	former := e.SeedCompany(t, "Former Employer", nil)
	contact := e.SeedContact(t, "Anna Weber", nil)
	left := time.Date(2021, 6, 30, 0, 0, 0, 0, time.UTC)
	employContact(t, e, contact, former, &left)
	employContact(t, e, contact, acme, nil)

	// Both reads, because they share one attach and a reader asking "who is
	// this and where do they work" asks the same question on either surface.
	row := listedContact(e.Admin(), t, e, contact)
	record, err := e.Contacts.GetContact(e.Admin(), ids.From[ids.ContactKind](contact), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the contact: %v", err)
	}
	for name, got := range map[string]*crmcontracts.ContactEmployer{
		"the list row": row.Employer,
		"the record":   record.Employer,
	} {
		if got == nil {
			t.Fatalf("%s carries no employer, want Acme", name)
		}
		// The account they work at today. A read that answered with the job
		// they left names a company this contact cannot be reached at.
		if ids.UUID(got.CompanyId) != acme || got.CompanyName != "Acme" {
			t.Errorf("%s names %s (%v), want Acme (%v)", name, got.CompanyName, got.CompanyId, acme)
		}
	}
}

// A job somebody has left names nobody: the read pairs the current-primary flag
// with the end date, so a leaver's old company does not go on standing as where
// they work.
func TestAContactWhoseOnlyEmploymentEndedNamesNoEmployer(t *testing.T) {
	e := Setup(t)
	former := e.SeedCompany(t, "Former Employer", nil)
	contact := e.SeedContact(t, "Left Last Year", nil)
	left := time.Date(2021, 6, 30, 0, 0, 0, 0, time.UTC)
	employContact(t, e, contact, former, &left)

	if got := listedContact(e.Admin(), t, e, contact).Employer; got != nil {
		t.Errorf("a leaver still names %s as their employer", got.CompanyName)
	}
}

// The employer is two disclosures, and losing either one loses the field and
// keeps the contact. Who works where is a fact about the PAIR, which the grant
// on the contact does not cover; the name is the account's own to disclose. A
// caller short of either still gets their contacts — the contract says an
// absent employer never means "works nowhere", which is what stops the omission
// being read as an answer.
func TestTheEmployerNeedsBothTheEdgeAndTheCompanyGrant(t *testing.T) {
	e := Setup(t)
	acme := e.SeedCompany(t, "Acme", nil)
	contact := e.SeedContact(t, "Anna Weber", nil)
	employContact(t, e, contact, acme, nil)

	for missing, grants := range map[string]map[string]principal.ObjectGrant{
		"relationship": {objContact: {Read: true}, objCompany: {Read: true}},
		"company":      {objContact: {Read: true}, objRelationship: {Read: true}},
	} {
		partial := e.As(e.AdminUser, nil, principal.Permissions{
			RoleKeys: []string{roleReadOnly},
			Objects:  grants,
			RowScope: principal.RowScopeAll,
		})
		if got := listedContact(partial, t, e, contact).Employer; got != nil {
			t.Errorf("without the %s grant the row still names %s", missing, got.CompanyName)
		}
	}
}
