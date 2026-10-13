// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A received message whose capture recorded no sender participant still shows
// who sent it: on a received message the counterparty is the sender.
func TestAReceivedMessageAlwaysNamesItsSender(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please send the report", "commitment", OwedVerdictAsksUs)
	// An older capture dropped a sender address the seat held, leaving no row.
	e.exec(t, `DELETE FROM activity_participant WHERE activity_id = $1 AND role = 'from'`, id)

	email, err := storeKnowing(e).GetEmailPresentation(e.as(), ids.From[ids.ActivityKind](id), nil)
	if err != nil {
		t.Fatalf("reading the message: %v", err)
	}
	if len(email.From) != 1 || email.From[0].Address != requestCounterparty {
		t.Fatalf("From reads %+v, want the sender %s", email.From, requestCounterparty)
	}
}

// The logged path keeps the stated header addresses beside the contacts and
// seats they resolved to. The envelope folds the two statements of one human
// back into one line, and leaves a bare address nobody resolved standing.
func TestTheEnvelopeNamesEachPartyOnce(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Buyer Contact', 'seed', 'system')`, contact)
	e.exec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, 'buyer@fold.test', 'seed', 'system')`, contact)
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'hello', now(), 'seed', 'system')`, activity)
	for _, row := range []struct {
		role, address string
		contact, user *ids.UUID
	}{
		{role: "from", contact: &contact},
		{role: "from", address: "buyer@fold.test"},
		{role: "to", user: &e.rep},
		{role: "to", address: "rep-" + e.rep.String() + "@load.test"},
		{role: "to", address: "stranger@fold.test"},
	} {
		e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, contact_id, user_id, address)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))`,
			ids.NewV7(), activity, row.role, row.contact, row.user, row.address)
	}

	email, err := storeKnowing(e).GetEmailPresentation(e.as(), ids.From[ids.ActivityKind](activity), nil)
	if err != nil {
		t.Fatalf("reading the message: %v", err)
	}
	if len(email.From) != 1 || email.From[0].Address != "buyer@fold.test" || email.From[0].ContactId == nil {
		t.Fatalf("From reads %+v, want the one contact carrying their own address", email.From)
	}
	if len(email.To) != 2 {
		t.Fatalf("To reads %+v, want the seat once and the unresolved stranger", email.To)
	}
	if email.To[0].UserId == nil || email.To[0].Address != "rep-"+e.rep.String()+"@load.test" {
		t.Fatalf("To reads %+v, want the seat first with their own address", email.To)
	}
	if email.To[1].Address != "stranger@fold.test" {
		t.Fatalf("To reads %+v, want the bare stranger kept", email.To)
	}
}

// A sender writing from their SECOND address still folds: the contact row
// borrows the address the message stated, not their primary one.
func TestTheFoldRecognisesASecondaryAddress(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Buyer Contact', 'seed', 'system')`, contact)
	e.exec(t, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, 'primary@fold.test', true, 'seed', 'system'),
		       ($1, 'second@fold.test', false, 'seed', 'system')`, contact)
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'hello', now(), 'seed', 'system')`, activity)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, contact_id)
		VALUES ($1, $2, 'from', $3)`, ids.NewV7(), activity, contact)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', 'second@fold.test')`, ids.NewV7(), activity)

	email, err := storeKnowing(e).GetEmailPresentation(e.as(), ids.From[ids.ActivityKind](activity), nil)
	if err != nil {
		t.Fatalf("reading the message: %v", err)
	}
	if len(email.From) != 1 || email.From[0].Address != "second@fold.test" || email.From[0].ContactId == nil {
		t.Fatalf("From reads %+v, want the one contact carrying the address the message stated", email.From)
	}
}

// A logged email that states its headers lists exactly the parties they name.
// A contact the logger linked but no header names keeps its participant row as
// evidence of the conversation, and the envelope does not list them.
func TestTheEnvelopeListsOnlyThePartiesTheHeadersName(t *testing.T) {
	e := setupLoad(t)
	sender, from := e.mailContact(t, "sender", "workspace")
	firstCc, firstCcAddress := e.mailContact(t, "first-cc", "workspace")
	secondCc, secondCcAddress := e.mailContact(t, "second-cc", "workspace")
	unstated, _ := e.mailContact(t, "unstated", "workspace")
	source := seedEmailRequestFiledUnder(t, e, "Send the report", "commitment", OwedVerdictAsksUs, from,
		[]ActivityLinkInput{contactLink(sender), contactLink(firstCc), contactLink(secondCc), contactLink(unstated)},
		mailParticipants{From: from, Cc: []string{firstCcAddress, secondCcAddress}})

	email, err := storeKnowing(e).GetEmailPresentation(e.as(), ids.From[ids.ActivityKind](source), nil)
	if err != nil {
		t.Fatalf("reading the message: %v", err)
	}
	if got := partyContacts(email.From); len(got) != 1 || !got[sender] {
		t.Fatalf("From reads %+v, want only the sender %v", email.From, sender)
	}
	if got := partyContacts(email.Cc); len(got) != 2 || !got[firstCc] || !got[secondCc] {
		t.Fatalf("Cc reads %+v, want the two copied contacts %v and %v", email.Cc, firstCc, secondCc)
	}
	for _, header := range [][]crmcontracts.EmailParty{email.From, email.To, email.Cc, email.Bcc} {
		if partyContacts(header)[unstated] {
			t.Fatalf("the envelope lists %v, a contact no header names: %+v", unstated, header)
		}
	}
	var rows int
	if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND contact_id = $2`, source, unstated).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("the unstated contact has %d participant rows, want its one row of evidence", rows)
	}
}

// A logged email that states no header knows its parties only through the
// contacts linked to it, so those contacts are its envelope.
func TestALoggedEmailWithNoHeadersListsItsLinkedContacts(t *testing.T) {
	e := setupLoad(t)
	contact, _ := e.mailContact(t, "linked", "workspace")
	subject, direction := "hello", "inbound"
	logged, _, err := storeKnowing(e).LogActivity(e.asSeat(e.rep), LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, Source: "manual",
		Links: []ActivityLinkInput{contactLink(contact)},
	})
	if err != nil {
		t.Fatal(err)
	}

	email, err := storeKnowing(e).GetEmailPresentation(e.as(), ids.From[ids.ActivityKind](ids.UUID(logged.Id)), nil)
	if err != nil {
		t.Fatalf("reading the message: %v", err)
	}
	if got := partyContacts(email.From); len(got) != 1 || !got[contact] {
		t.Fatalf("From reads %+v, want the linked contact %v", email.From, contact)
	}
}

// A promise in a sent email is made to whom the headers address. A contact
// the logger linked but no header names is not a recipient of it.
func TestASentMessageIsAddressedOnlyToTheContactsItsHeadersName(t *testing.T) {
	e := setupLoad(t)
	named, namedAddress := e.mailContact(t, "named", "workspace")
	unstated, _ := e.mailContact(t, "unstated", "workspace")
	subject, direction := "the offer", "outbound"
	logged, _, err := storeKnowing(e).LogActivity(e.asSeat(e.rep), LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, Source: "manual",
		Links:   []ActivityLinkInput{contactLink(named), contactLink(unstated)},
		EmailTo: []string{namedAddress, "stranger@customer.test"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var parties MessageParties
	ctx := e.as()
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		parties, err = storeKnowing(e).PartiesOf(ctx, tx, ids.UUID(logged.Id))
		return err
	}); err != nil {
		t.Fatalf("reading the message's parties: %v", err)
	}
	if len(parties.RecipientContacts) != 1 || parties.RecipientContacts[0] != named {
		t.Fatalf("recipients read %v, want only the addressed contact %v", parties.RecipientContacts, named)
	}
}

// A contact the headers named stays a party after the address they were named
// at is archived: the header said what it said when the mail was logged.
func TestAnArchivedAddressStillNamesItsContactOnTheHeader(t *testing.T) {
	e := setupLoad(t)
	named, namedAddress := e.mailContact(t, "named", "workspace")
	subject, direction := "the offer", "outbound"
	logged, _, err := storeKnowing(e).LogActivity(e.asSeat(e.rep), LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, Source: "manual",
		Links:   []ActivityLinkInput{contactLink(named)},
		EmailTo: []string{namedAddress},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, named)

	email, err := storeKnowing(e).GetEmailPresentation(e.as(), ids.From[ids.ActivityKind](ids.UUID(logged.Id)), nil)
	if err != nil {
		t.Fatalf("reading the message: %v", err)
	}
	if !partyContacts(email.To)[named] {
		t.Fatalf("To reads %+v, want the contact the header named %v", email.To, named)
	}
	var parties MessageParties
	ctx := e.as()
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		parties, err = storeKnowing(e).PartiesOf(ctx, tx, ids.UUID(logged.Id))
		return err
	}); err != nil {
		t.Fatalf("reading the message's parties: %v", err)
	}
	if len(parties.RecipientContacts) != 1 || parties.RecipientContacts[0] != named {
		t.Fatalf("recipients read %v, want the contact the header named %v", parties.RecipientContacts, named)
	}
}

func partyContacts(parties []crmcontracts.EmailParty) map[ids.UUID]bool {
	named := map[ids.UUID]bool{}
	for _, p := range parties {
		if p.ContactId != nil {
			named[ids.UUID(*p.ContactId)] = true
		}
	}
	return named
}
