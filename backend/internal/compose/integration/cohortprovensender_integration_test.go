// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The cohort repair names a contact the sender of filed mail only where the
// stated address proves them. They hold it, and no other contact ever held it.
// The tests run the consumer, which is how production wires the rule.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// strandedInbound writes received mail capture left on no record, with a sender
// row that names only the address when stated is true.
func strandedInbound(t *testing.T, e *Env, address string, stated bool) ids.UUID {
	t.Helper()
	mail := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, direction, occurred_at, source, captured_by, counterparty_email)
		VALUES ($1, 'email', 'hello', 'inbound', now() - interval '1 day', 'gmail', 'connector:gmail', $2)`, mail, address)
	if stated {
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'from', $2)`, mail, address)
	}
	return mail
}

func contactWithAddress(t *testing.T, e *Env, name, address string, archived bool) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, name, &e.Rep1)
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by, archived_at)
		VALUES ($1, $2, 'manual', 'human:seed', CASE WHEN $3 THEN now() END)`, contact, address, archived)
	return contact
}

func repairCohortOf(t *testing.T, e *Env, contact ids.UUID) {
	t.Helper()
	gen := compose.NewCohortPromoteGen(e.Pool, contacts.NewStore(e.DB()), slog.Default())
	if err := gen.HandleEvent(context.Background(), kevents.Envelope{
		EventID: ids.NewV7(), Type: "contact.updated", Version: 1, OccurredAt: clocktest.Now(t),
		Entity: kevents.EntityRef{Type: "contact", ID: contact}, Trace: kevents.Trace{CorrelationID: ids.NewV7()},
	}); err != nil {
		t.Fatalf("repairing the cohort: %v", err)
	}
}

func sentBy(t *testing.T, e *Env, mail, contact ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity_participant
		WHERE activity_id = $1 AND role = 'from' AND contact_id = $2`, mail, contact) > 0
}

func TestTheCohortRepairDoesNotMakeTheNewHolderOfAMovedAddressTheSender(t *testing.T) {
	e := Setup(t)
	const moved = "moved.inbox@customer.test"
	contactWithAddress(t, e, "Former Holder", moved, true)
	holder := contactWithAddress(t, e, "New Holder", moved, false)
	mail := strandedInbound(t, e, moved, true)

	repairCohortOf(t, e, holder)

	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, mail, holder) != 1 {
		t.Fatal("the repair did not file the mail under the address's holder, so the sender check below proves nothing")
	}
	if sentBy(t, e, mail, holder) {
		t.Error("the repair named the new holder of a moved address as the sender of mail it did not prove")
	}
	owed, err := e.Contacts.ContactsOwedACohortRepair(e.Admin(), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, contact := range owed {
		if contact.UUID == holder {
			t.Error("the sweep still offers the holder a promotion the repair refuses, so it never drains")
		}
	}
}

func TestTheCohortRepairNamesTheProvenSender(t *testing.T) {
	e := Setup(t)
	const address = "proven.writer@customer.test"
	writer := contactWithAddress(t, e, "Proven Writer", address, false)
	stated := strandedInbound(t, e, address, true)
	senderless := strandedInbound(t, e, address, false)

	repairCohortOf(t, e, writer)

	if !sentBy(t, e, stated, writer) {
		t.Error("the repair left the proven writer off the sender row that states their address")
	}
	if !sentBy(t, e, senderless, writer) {
		t.Error("the repair filed mail with no sender row under its proven writer without naming them the sender")
	}
}
