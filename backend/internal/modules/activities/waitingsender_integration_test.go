// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// buyerAndRepContacts seeds a buyer and the rep's own contact record, and
// returns them with the rep's id sorting FIRST by text, so a pick that ignores
// who wrote would name the rep.
func (e *loadEnv) buyerAndRepContacts(t *testing.T) (buyer, rep ids.UUID) {
	t.Helper()
	first, second := ids.NewV7(), ids.NewV7()
	if second.String() < first.String() {
		first, second = second, first
	}
	rep, buyer = first, second
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Boris Buyer', $2, 'seed', 'system')`, buyer, e.other)
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Rep Themselves', $2, 'seed', 'system')`, rep, e.rep)
	e.exec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, $2, 'seed', 'system')`, buyer, e.buyerAddress())
	e.exec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, $2, 'seed', 'system')`, rep, "rep-"+e.rep.String()+"@load.test")
	return buyer, rep
}

// buyerAddress is unique per test: contact_email is deduplicated installation-wide.
func (e *loadEnv) buyerAddress() string { return "boris-" + e.rep.String() + "@customer.test" }

func (e *loadEnv) mailFiledUnderBoth(t *testing.T, subject string, buyer, rep ids.UUID, senderContact *ids.UUID) ids.UUID {
	t.Helper()
	activity := e.seedWait(t, subject, "contact_id", rep)
	e.exec(t, `UPDATE activity_participant SET address = $3, contact_id = $2
		WHERE activity_id = $1 AND role = 'from'`, activity, senderContact, e.buyerAddress())
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), activity, buyer)
	return activity
}

// The buyer wrote to the rep, and both have a record the message is filed
// under. The row names the buyer, and the buyer's owner owes the reply.
func TestAWaitingRowNamesWhoWroteNotWhoReceived(t *testing.T) {
	e := setupLoad(t)
	buyer, rep := e.buyerAndRepContacts(t)
	activity := e.mailFiledUnderBoth(t, "Anfrage Entwicklungsteam", buyer, rep, &buyer)

	got := e.waitFor(t, activity)
	if got.ContactID != buyer {
		t.Fatalf("the row names contact %v, want the sender %v (the rep's own record is %v)", got.ContactID, buyer, rep)
	}
	if got.OwnerID != e.other {
		t.Fatalf("the reply is owed by %v, want the buyer's owner %v", got.OwnerID, e.other)
	}
}

// Without a resolved sender, a contact that is somebody's seat is still the
// last choice: the rep's own record is never the buyer.
func TestAWaitingRowNeverNamesASeatWhenACustomerIsFiled(t *testing.T) {
	e := setupLoad(t)
	buyer, rep := e.buyerAndRepContacts(t)
	activity := e.mailFiledUnderBoth(t, "Question without a resolved sender", buyer, rep, nil)

	if got := e.waitFor(t, activity); got.ContactID != buyer {
		t.Fatalf("the row names contact %v, want the customer %v rather than the rep's record %v", got.ContactID, buyer, rep)
	}
}

// Two customers on one message, neither a seat: the one who wrote is named,
// not the colleague of theirs on copy whose record happens to sort first.
func TestAWaitingRowNamesTheSenderAmongCustomers(t *testing.T) {
	e := setupLoad(t)
	buyer, copied := e.buyerAndRepContacts(t)
	e.exec(t, `DELETE FROM contact_email WHERE contact_id = $1`, copied)
	activity := e.mailFiledUnderBoth(t, "Question with a colleague on copy", buyer, copied, &buyer)

	if got := e.waitFor(t, activity); got.ContactID != buyer {
		t.Fatalf("the row names contact %v, want the sender %v rather than the copied customer %v",
			got.ContactID, buyer, copied)
	}
}
