// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

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
