// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A rep writes to an address no record holds. Before the consent engine is
// asked, the send makes a minimal contact owned by the sender. A held address
// keeps its contact or lead.
//
// An installation holds one workspace in its own database. Another tenant's
// records are not in the database these queries read. Inside one installation
// a matcher can still make a duplicate, convert a lead or move an owner.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type newAddressEnv struct {
	*apptest.AppEnv
	company string
	sender  string
}

func setupNewAddress(t *testing.T) *newAddressEnv {
	t.Helper()
	e := apptest.SetupAppWithOptions(t, compose.WithOperatorMail(apptest.DiscardingMailer{}))
	apptest.BootstrapWorkspaceSession(t, e, "New Address", "rep@newaddress.test", "Rep")
	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{
		"source": "manual", "display_name": "Buyer GmbH",
	}, nil, &company); status != http.StatusCreated {
		t.Fatalf("company create → %d", status)
	}
	var sender string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id::text FROM app_user WHERE email = 'rep@newaddress.test'`).Scan(&sender); err != nil {
		t.Fatalf("reading the sender's seat: %v", err)
	}
	return &newAddressEnv{AppEnv: e, company: company.ID, sender: sender}
}

type newAddressPreview struct {
	Allowed    bool `json:"allowed"`
	Recipients []struct {
		Address    string `json:"address"`
		ReasonCode string `json:"reason_code"`
		NewContact *bool  `json:"new_contact"`
	} `json:"recipients"`
}

func (e *newAddressEnv) message(to string) AnyMap {
	return AnyMap{
		"subject": "Your enquiry", "body": "Thanks for asking.",
		"to":    []string{to},
		"links": []AnyMap{{"entity_type": "company", "entity_id": e.company}},
	}
}

func (e *newAddressEnv) preview(t *testing.T, to string) newAddressPreview {
	t.Helper()
	var answer newAddressPreview
	if status := e.Call(t, "POST", "/v1/emails:preview", AnyMap{
		"to":    []string{to},
		"links": []AnyMap{{"entity_type": "company", "entity_id": e.company}},
	}, nil, &answer); status != http.StatusOK {
		t.Fatalf("preview → %d, want 200", status)
	}
	if len(answer.Recipients) != 1 {
		t.Fatalf("preview answered about %d recipients, want 1", len(answer.Recipients))
	}
	return answer
}

// send answers whether the message went. A refusal by the consent engine is an
// answer too; only a refusal of the address itself fails the test.
func (e *newAddressEnv) send(t *testing.T, to string) bool {
	t.Helper()
	var problem problemBody
	status := e.Call(t, "POST", "/v1/emails", e.message(to), nil, &problem)
	if status == http.StatusUnprocessableEntity {
		t.Fatalf("send to %s → 422 %+v, want the address accepted", to, problem.Details.Errors)
	}
	return status == http.StatusAccepted
}

// contactsHolding lists the owner of each live contact holding address.
func (e *newAddressEnv) contactsHolding(t *testing.T, address string) []string {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(), `
		SELECT coalesce(c.owner_id::text, '') FROM contact c
		  JOIN contact_email ce ON ce.contact_id = c.id AND ce.archived_at IS NULL
		 WHERE ce.email = $1 AND c.archived_at IS NULL`, address)
	if err != nil {
		t.Fatalf("reading the contacts holding %s: %v", address, err)
	}
	defer rows.Close()
	var owners []string
	for rows.Next() {
		var owner string
		if err := rows.Scan(&owner); err != nil {
			t.Fatal(err)
		}
		owners = append(owners, owner)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return owners
}

// The preview answers about the contact the send will make, and makes none.
// The send then makes one, owned by the sender. The engine's answer about it
// is the one the preview gave. A second send reuses it.
//
// Mutation: drop the fileTypedRecipients call from PrepareSend and the send
// answers 422 recipient_not_on_file. Commit the preview's transaction and the
// contact exists before anybody pressed Send.
func TestASendToANewAddressMakesOneContactOwnedByTheSender(t *testing.T) {
	e := setupNewAddress(t)
	const address = "new.buyer@prospect.test"

	answer := e.preview(t, "New.Buyer@Prospect.test")
	if got := answer.Recipients[0]; got.NewContact == nil || !*got.NewContact {
		t.Fatalf("preview of a new address = %+v, want new_contact", got)
	}
	if got := answer.Recipients[0].ReasonCode; got == "recipient_resolves_to_no_single_subject" {
		t.Fatalf("preview answered %q, want the answer about the contact the send makes", got)
	}
	if owners := e.contactsHolding(t, address); len(owners) != 0 {
		t.Fatalf("the preview left %d contact(s) behind, want none", len(owners))
	}

	if sent := e.send(t, "New.Buyer@Prospect.test"); sent != answer.Allowed {
		t.Errorf("the send went=%v and the preview said allowed=%v — they answer about one contact", sent, answer.Allowed)
	}
	owners := e.contactsHolding(t, address)
	if len(owners) != 1 || owners[0] != e.sender {
		t.Fatalf("contacts holding the new address = %v, want one owned by the sender %s", owners, e.sender)
	}

	if again := e.preview(t, address); again.Recipients[0].NewContact != nil {
		t.Errorf("preview of the filed address still says new_contact=%v", *again.Recipients[0].NewContact)
	}
	e.send(t, address)
	if owners := e.contactsHolding(t, address); len(owners) != 1 {
		t.Errorf("a second send left %d contacts holding the address, want the one it reused", len(owners))
	}
}

// An address a contact already holds is that contact: no second record, and
// its owner stays as it was, here none at all.
//
// Mutation: skip the identity check in typedRecipientFileable and the create
// refuses the claimed address, which fails the send.
func TestASendToAKnownAddressReusesTheContact(t *testing.T) {
	e := setupNewAddress(t)
	const address = "known@customer.test"
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Known Customer",
		"emails": []AnyMap{{"email": address}},
	}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact → %d", status)
	}
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE contact SET owner_id = NULL WHERE id = $1`, contact.ID); err != nil {
		t.Fatalf("clearing the owner: %v", err)
	}

	if got := e.preview(t, address).Recipients[0].NewContact; got != nil {
		t.Errorf("preview of a known address says new_contact=%v", *got)
	}
	e.send(t, address)
	if owners := e.contactsHolding(t, address); len(owners) != 1 || owners[0] != "" {
		t.Errorf("contacts holding the known address = %v, want the one unowned contact", owners)
	}
}

// A lead holding the address is the recipient. The send makes no contact and
// converts nobody: somebody is a lead or a contact, and only a human converts.
func TestASendToALeadsAddressLeavesTheLeadALead(t *testing.T) {
	e := setupNewAddress(t)
	const address = "lead@prospect.test"
	var lead struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"full_name": "Prospect Lead", "email": address, "source": "manual",
	}, nil, &lead); status != http.StatusCreated {
		t.Fatalf("create lead → %d", status)
	}

	if got := e.preview(t, address).Recipients[0].NewContact; got != nil {
		t.Errorf("preview of a lead's address says new_contact=%v", *got)
	}
	e.send(t, address)
	if owners := e.contactsHolding(t, address); len(owners) != 0 {
		t.Errorf("the send made %d contact(s) for a lead's address, want none", len(owners))
	}
	var live bool
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT archived_at IS NULL FROM lead WHERE id = $1`, lead.ID).Scan(&live); err != nil {
		t.Fatalf("reading the lead: %v", err)
	}
	if !live {
		t.Error("the lead was archived by a send, want it left a lead")
	}
}
