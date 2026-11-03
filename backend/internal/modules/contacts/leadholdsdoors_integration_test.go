// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The contact doors besides New contact. Each refuses a write that would make a
// live lead a contact too. The ones a user types into convert when asked to.

import (
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestQuickCaptureOfALiveLeadIsRefusedAndItsRetryConverts(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Quinn Lead"), Email: new("quinn@lead.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	in := QuickCaptureInput{FullName: "Quinn Lead", Email: new("Quinn@Lead.test")}
	_, err = e.store.QuickCapture(ctx, in)
	if held := leadHeldRefusal(t, err); held.Field != "email" || len(held.LeadIDs) != 1 || held.LeadIDs[0] != leadID {
		t.Fatalf("quick capture of a lead's address: %+v", held)
	}
	e.assertNoContactHolds(ctx, t, "quinn@lead.test")
	in.ConvertLeadIDs = []ids.LeadID{leadID}
	out, err := e.store.QuickCapture(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if status, target, _, _ := e.leadOutcome(ctx, t, leadID.UUID); status != "promoted" || target == nil || *target != ids.UUID(out.Contact.Id) {
		t.Fatalf("the retry did not convert the lead: %s, %v", status, target)
	}
}

func TestQuickCaptureOfALeadsProfileNamesTheProfileField(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if _, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Pia Profile"), LinkedInURL: new("https://linkedin.com/in/pia-profile"), Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	_, err := e.store.QuickCapture(ctx, QuickCaptureInput{FullName: "Pia Profile", ProfileURL: new("linkedin.com/in/pia-profile")})
	if held := leadHeldRefusal(t, err); held.Field != profileURLRequestField {
		t.Fatalf("quick capture of a lead's profile: %+v", held)
	}
}

func TestAnEmailEditToALiveLeadsAddressIsRefusedAndItsRetryMergesTheLead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contact, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Edda Edit", Source: "manual", Emails: []ContactEmailInput{{Email: "edda@old.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Edda Edit"), Email: new("edda@new.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	edit := UpdateContactInput{Source: manualSource, Emails: []ContactEmailInput{
		{Email: "edda@old.test", EmailType: "work", IsPrimary: true}, {Email: "edda@new.test", EmailType: "work"},
	}}
	_, err = e.store.UpdateContact(ctx, contactID, edit)
	if held := leadHeldRefusal(t, err); len(held.LeadIDs) != 1 || held.LeadIDs[0] != leadID || !held.Convertible {
		t.Fatalf("an edit to a lead's address: %+v", held)
	}
	e.assertNoContactHolds(ctx, t, "edda@new.test")
	edit.ConvertLeadIDs = []ids.LeadID{leadID}
	if _, err := e.store.UpdateContact(ctx, contactID, edit); err != nil {
		t.Fatal(err)
	}
	if status, target, archived, _ := e.leadOutcome(ctx, t, leadID.UUID); status != "promoted" || !archived || target == nil || *target != contactID.UUID {
		t.Fatalf("the retry did not merge the lead into this contact: %s,%v,%v", status, target, archived)
	}
}

func TestAnEditOfAContactThatAlreadySharesALeadsAddressStillSaves(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contact, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Paula Pair", Source: "manual", Emails: []ContactEmailInput{{Email: "paula@pair.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Paula Pair"), Email: new("paula@seed.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE lead SET email = 'paula@pair.test' WHERE id = $1`, lead.Id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(contact.Id)), UpdateContactInput{
		Source: manualSource, Title: new("Head of Sales"),
		Emails: []ContactEmailInput{{Email: "paula@pair.test", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("an edit that gives the contact no new key: %v", err)
	}
}

func TestRestoringAContactWhoseAddressALiveLeadNowHoldsIsRefused(t *testing.T) {
	e := setupDedupe(t)
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	actor.Permissions.Objects[contactEntity] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	ctx := principal.WithActor(e.as(), actor)
	contact, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Rhea Restore", Source: "manual", Emails: []ContactEmailInput{{Email: "rhea@restore.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if _, err := e.store.ArchiveContact(ctx, contactID, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Rhea Restore"), Email: new("rhea@restore.test"), Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	err = e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := e.store.RestoreContactTx(ctx, tx, contactID, nil, storekit.RestoreWith{})
		return err
	})
	refusal, ok := errors.AsType[*storekit.RestoreRefusal](err)
	if !ok || refusal.Reason != storekit.RestoreValueTaken || refusal.Detail != CodeLeadHoldsEmail {
		t.Fatalf("restore over a live lead: %v", err)
	}
}

func TestAVCardForALiveLeadCreatesNothingAndNamesTheLead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Vera Card"), Email: new("vera@card.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	results := importCards(ctx, t, e, "BEGIN:VCARD\nFN:Vera Card\nEMAIL;TYPE=WORK:vera@card.test\nEND:VCARD\n")
	if len(results) != 1 || results[0].Outcome != VCardSkipped || results[0].LeadID == nil || results[0].LeadID.UUID != ids.UUID(lead.Id) {
		t.Fatalf("a card for a live lead: %+v", results)
	}
	e.assertNoContactHolds(ctx, t, "vera@card.test")
}

func TestTheContactWriterRefusesALiveLeadsKeysUnlessItConvertsThatLead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Wren Writer"), Email: new("wren@writer.test"), LinkedInURL: new("https://linkedin.com/in/wren-writer"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	write := func(spec ContactSpec) error {
		return e.store.tx(ctx, func(tx pgx.Tx) error {
			_, err := createContact(ctx, tx, e.store, ContactResolution{Decision: DecisionNoMatch}, spec)
			return err
		})
	}
	byEmail := ContactSpec{FullName: "Wren Writer", Source: "manual", CapturedBy: "human:x", Emails: []ContactEmailInput{{Email: "wren@writer.test", EmailType: "work", IsPrimary: true}}}
	byProfile := ContactSpec{FullName: "Wren Writer", Source: "manual", CapturedBy: "human:x", Social: map[string]any{SocialLinkedIn: "https://linkedin.com/in/wren-writer"}}
	for name, spec := range map[string]ContactSpec{"email": byEmail, "profile": byProfile} {
		if held := leadHeldRefusal(t, write(spec)); len(held.LeadIDs) != 1 || held.LeadIDs[0] != leadID {
			t.Fatalf("the writer over a lead's %s: %+v", name, held)
		}
	}
	byEmail.converting = []ids.LeadID{leadID}
	if err := write(byEmail); err != nil {
		t.Fatalf("the writer converting the lead that holds the address: %v", err)
	}
}
