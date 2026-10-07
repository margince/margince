// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A lead worked from a contact copied that contact's name, title and employer.
// With no address to match it by, only the link finds it, so erasing the
// contact has to follow the link or the copy outlives the erasure.
func TestErasingAContactAnonymizesTheLeadWorkedFromIt(t *testing.T) {
	e := Setup(t)
	title := "Head of Logistics"
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Dana Example", Title: &title, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "manual", FromContactID: &contactID})
	if err != nil {
		t.Fatalf("working the contact as a lead: %v", err)
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), ids.UUID(contact.Id), "test"); err != nil {
		t.Fatalf("erasing the contact: %v", err)
	}

	var name string
	var leadTitle *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT full_name, title FROM lead WHERE id = $1`, lead.Id).
			Scan(&name, &leadTitle)
	}); err != nil {
		t.Fatalf("reading the lead back: %v", err)
	}
	if name == "Dana Example" || leadTitle != nil {
		t.Errorf("the lead worked from the erased contact still reads %q, %v", name, leadTitle)
	}
}

// Merging the contact a lead was worked from carries the link to the survivor,
// so the survivor's page offers that lead instead of a second one.
func TestAMergedContactsLeadIsWorkedFromTheSurvivor(t *testing.T) {
	e := Setup(t)
	source, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Dana Example", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the merged-away contact: %v", err)
	}
	target, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Dana Example", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the survivor: %v", err)
	}
	sourceID := ids.From[ids.ContactKind](ids.UUID(source.Id))
	targetID := ids.From[ids.ContactKind](ids.UUID(target.Id))
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "manual", FromContactID: &sourceID})
	if err != nil {
		t.Fatalf("working the merged-away contact as a lead: %v", err)
	}

	if _, err := e.Contacts.MergeContact(e.Admin(), sourceID, targetID, nil); err != nil {
		t.Fatalf("merging the contacts: %v", err)
	}

	_, _, err = e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "manual", FromContactID: &targetID})
	var refused *contacts.DuplicateContactLeadError
	if !errors.As(err, &refused) || refused.ExistingID.UUID != ids.UUID(lead.Id) {
		t.Fatalf("a lead from the survivor = %v, want the refusal naming the lead worked from the merged-away contact", err)
	}
}

// A lead merged into another hands over the contact it was worked from, so the
// surviving lead is the one that contact is worked through.
func TestAMergedLeadHandsItsContactToTheSurvivor(t *testing.T) {
	e := Setup(t)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Dana Example", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	worked, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "manual", FromContactID: &contactID})
	if err != nil {
		t.Fatalf("working the contact as a lead: %v", err)
	}
	name, email := "Dana Example", "dana@contoso.example"
	survivor, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "import", FullName: &name, Email: &email})
	if err != nil {
		t.Fatalf("creating the surviving lead: %v", err)
	}

	merged, err := e.Contacts.MergeLead(e.Admin(),
		ids.From[ids.LeadKind](ids.UUID(worked.Id)), ids.From[ids.LeadKind](ids.UUID(survivor.Id)))
	if err != nil {
		t.Fatalf("merging the leads: %v", err)
	}
	if merged.FromContactId == nil || ids.UUID(*merged.FromContactId) != ids.UUID(contact.Id) {
		t.Errorf("the surviving lead is worked from %v, want the contact %s", merged.FromContactId, contact.Id)
	}
}
