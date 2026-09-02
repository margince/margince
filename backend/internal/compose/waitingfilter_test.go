// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the waiting seam REFUSES to call a customer.
//
// Both cases came off the live page: a rep's day opened with e-signature
// notifications, a shared-folder notice and a flight confirmation, and the same
// signature request twice. Neither is a customer waiting, and a queue that says
// they are teaches a rep to stop reading it.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
)

func TestAMachineSenderIsNotACustomerWaiting(t *testing.T) {
	rows := []activities.WaitingReply{
		{Subject: "E-Signatur-Anforderung", Sender: "noreply@docusign.net"},
		{Subject: "Folder shared with you", Sender: "notifications@dropbox.com"},
		{Subject: "Re: the retrofit quote", Sender: "anna.weber@acme.com"},
	}

	kept := keepWaitingCustomers(rows)

	if len(kept) != 1 {
		t.Fatalf("kept %d of three rows, wanted only the person", len(kept))
	}
	if kept[0].Subject != "Re: the retrofit quote" {
		t.Fatalf("kept %q, wanted the customer", kept[0].Subject)
	}
}

// Two customers can write the same words. Folding on the subject alone made
// the second one vanish with nothing on the page to say so.
func TestTwoCustomersSharingASubjectAreTwoWaits(t *testing.T) {
	rows := []activities.WaitingReply{
		{Subject: "Re: proposal", Sender: "anna@a.example"},
		{Subject: "Re: proposal", Sender: "bob@b.example"},
	}

	if kept := keepWaitingCustomers(rows); len(kept) != 2 {
		t.Fatalf("two customers writing the same subject collapsed to %d", len(kept))
	}
}

func TestTheSameSubjectIsOneRowHoweverManyThreadsCarriedIt(t *testing.T) {
	at := time.Now()
	rows := []activities.WaitingReply{
		{Subject: "Signature required", Sender: "anna@acme.com", OccurredAt: at},
		{Subject: "Signature required", Sender: "anna@acme.com", OccurredAt: at.Add(time.Hour)},
	}

	if kept := keepWaitingCustomers(rows); len(kept) != 1 {
		t.Fatalf("one subject produced %d rows", len(kept))
	}
}

// A message with no subject is not folded away with the others: several
// untitled messages are several waits, and collapsing them would hide all but
// one customer behind an empty string.
func TestUntitledMessagesAreNotFoldedIntoOne(t *testing.T) {
	rows := []activities.WaitingReply{
		{Sender: "anna@acme.com"},
		{Sender: "bob@acme.com"},
	}

	if kept := keepWaitingCustomers(rows); len(kept) != 2 {
		t.Fatalf("two untitled waits collapsed to %d", len(kept))
	}
}
