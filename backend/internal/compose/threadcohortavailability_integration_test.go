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

func TestACohortSweepSkipsARepairingContactUntilPublication(t *testing.T) {
	e := integration.Setup(t)
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil)
	type participant struct {
		source  ids.UUID
		contact ids.ContactID
	}
	var capturedContacts []participant
	for _, candidateContact := range []struct{ name, address string }{{"Bob Buyer", "bob@buyer.test"}, {"Casey Buyer", "casey@buyer.test"}} {
		source := capturePartiesMailUsing(t, e, sink, partyMailProof{outbound: true, sentByOwner: true}, candidateContact.name+" <"+candidateContact.address+">", "", "Could we discuss the product requirements?")
		if count := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND address=$2 AND contact_id IS NULL`, source.ID, candidateContact.address); count != 1 {
			t.Fatalf("native bare participant count=%d", count)
		}
		contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: candidateContact.name, Source: "manual", Emails: []contacts.ContactEmailInput{{Email: candidateContact.address, EmailType: "work", IsPrimary: true}}})
		if err != nil {
			t.Fatal(err)
		}
		capturedContacts = append(capturedContacts, participant{source: source.ID, contact: ids.From[ids.ContactKind](ids.UUID(contact.Id))})
	}
	if capturedContacts[0].contact.String() > capturedContacts[1].contact.String() {
		capturedContacts[0], capturedContacts[1] = capturedContacts[1], capturedContacts[0]
	}
	hidden, available := capturedContacts[0], capturedContacts[1]
	key := e.WsScalar(t, `SELECT thread_key FROM activity WHERE id=$1`, hidden.source)
	run := beginUnavailableThread(t, e, key)
	system := principal.SystemActing(e.Admin(), "system:test_cohort_availability")
	owed, err := e.Contacts.ContactsOwedACohortRepair(system, 1)
	if err != nil || len(owed) != 1 || owed[0] != available.contact {
		t.Fatalf("limit-one eligible contacts=%v: %v", owed, err)
	}
	if _, err := e.Contacts.RepairContactCohort(system, available.contact); err != nil {
		t.Fatal(err)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND contact_id=$2`, available.source, available.contact); count != 1 {
		t.Fatalf("available participant promotions=%d", count)
	}
	if owed, err := e.Contacts.ContactsOwedACohortRepair(system, 1); err != nil || len(owed) != 0 {
		t.Fatalf("blocked contact monopolized the empty page: %v %v", owed, err)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND contact_id=$2`, hidden.source, hidden.contact); count != 0 {
		t.Fatal("repairing source was prematurely promoted")
	}
	finishUnavailableThread(t, e, run)
	owed, err = e.Contacts.ContactsOwedACohortRepair(system, 1)
	if err != nil || len(owed) != 1 || owed[0] != hidden.contact {
		t.Fatalf("published pending contact did not return: %v %v", owed, err)
	}
	if _, err := e.Contacts.RepairContactCohort(system, hidden.contact); err != nil {
		t.Fatal(err)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND contact_id=$2`, hidden.source, hidden.contact); count != 1 {
		t.Fatalf("published participant promotions=%d", count)
	}
}
