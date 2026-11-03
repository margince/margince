// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A contact and a lead address written before the folded column existed get
// the same folded domain the writers store today.
func TestTheFoldTaskFoldsContactAndLeadAddressesWrittenBeforeTheColumn(t *testing.T) {
	e := integration.Setup(t)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Anna Brandt", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "anna@bücher.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: new("Ben Brandt"), Email: new("ben@BÜCHER.example"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	const contactFold = `SELECT coalesce(email_domain_folded, '') FROM contact_email WHERE contact_id = $1`
	const leadFold = `SELECT coalesce(email_domain_folded, '') FROM lead WHERE id = $1`
	if got := e.WsScalar(t, contactFold, contact.Id); got != "xn--bcher-kva.example" {
		t.Fatalf("the contact writer stored %q, want xn--bcher-kva.example", got)
	}
	e.WsExec(t, `UPDATE contact_email SET email_domain_folded = NULL WHERE contact_id = $1`, contact.Id)
	e.WsExec(t, `UPDATE lead SET email_domain_folded = NULL WHERE id = $1`, lead.Id)

	runMachineAddressTask(principal.SystemActing(e.Admin(), "system:datatask-test"), t, e, addressDomainFoldDataTask())

	if got := e.WsScalar(t, contactFold, contact.Id); got != "xn--bcher-kva.example" {
		t.Errorf("the fold task stored %q on the contact address, want xn--bcher-kva.example", got)
	}
	if got := e.WsScalar(t, leadFold, lead.Id); got != "xn--bcher-kva.example" {
		t.Errorf("the fold task stored %q on the lead, want xn--bcher-kva.example", got)
	}

	// An edit of the address moves its folded domain with it.
	if _, err := e.Contacts.UpdateLead(e.Admin(), ids.From[ids.LeadKind](ids.UUID(lead.Id)),
		contacts.UpdateLeadInput{Email: new("ben@münchen.example")}); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, leadFold, lead.Id); got != "xn--mnchen-3ya.example" {
		t.Errorf("an edited lead address stores %q, want xn--mnchen-3ya.example", got)
	}
}
