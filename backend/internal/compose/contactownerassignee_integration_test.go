// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Handing one contact or company to a colleague asks the rule a deal's owner
// change already asks (auth.EnsureAssignee), so a bulk reassignment and a
// single PATCH refuse the same colleague.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestHandingOneContactOrCompanyOnRefusesAColleagueWhoMayNotOwnRecords(t *testing.T) {
	e := integration.Setup(t)
	rep1 := ids.From[ids.UserKind](e.Rep1)
	rep3 := ids.From[ids.UserKind](e.Rep3)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Nora Lind", OwnerID: &rep1})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Lind AB", OwnerID: &rep1, Source: "manual"})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	e.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.Rep3)

	_, contactErr := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](ids.UUID(contact.Id)),
		contacts.UpdateContactInput{OwnerID: &rep3})
	_, companyErr := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](ids.UUID(company.Id)),
		contacts.UpdateCompanyInput{OwnerID: &rep3})
	for name, err := range map[string]error{"contact": contactErr, "company": companyErr} {
		if !errors.As(err, new(*auth.AssigneeNotAllowedError)) {
			t.Errorf("handing the %s to a suspended colleague → %v, want the assignee refusal", name, err)
		}
	}

	// Sending back the owner a record already has hands nothing on, so a save
	// of a record whose owner has since been suspended still goes through.
	e.WsExec(t, `UPDATE app_user SET status = 'active' WHERE id = $1`, e.Rep3)
	owned, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Oskar Lind", OwnerID: &rep3})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	e.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.Rep3)
	title := "Buyer"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](ids.UUID(owned.Id)),
		contacts.UpdateContactInput{OwnerID: &rep3, Title: &title}); err != nil {
		t.Errorf("a save echoing the unchanged owner → %v, want it accepted", err)
	}

	// A new record may still name a colleague who has been invited and has not
	// signed in yet.
	e.WsExec(t, `UPDATE app_user SET status = 'invited' WHERE id = $1`, e.Rep3)
	if _, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Pia Lind", OwnerID: &rep3}); err != nil {
		t.Errorf("creating a contact for an invited colleague → %v, want it accepted", err)
	}
}
