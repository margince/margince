// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// logMailFrom logs one inbound mail from an address, filed under a contact.
func logMailFrom(t *testing.T, e *Env, address string, filedUnder ids.UUID) ids.UUID {
	t.Helper()
	logged, _, err := e.Activities.LogActivity(e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms), activities.LogActivityInput{
		Kind: "email", Subject: new("Contract question"), Body: new("Can we talk?"), Direction: new("inbound"),
		EmailFrom: address,
		Links:     []activities.ActivityLinkInput{{EntityType: "contact", EntityID: filedUnder}},
	})
	if err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	return ids.UUID(logged.Id)
}

func giveAddress(t *testing.T, e *Env, contact ids.UUID, address string) {
	t.Helper()
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("giving %v the address %s: %v", contact, address, err)
	}
}

func namedContact(author *crmcontracts.ActivityAuthor) ids.UUID {
	if author == nil || author.ContactId == nil {
		return ids.Nil
	}
	return ids.UUID(*author.ContactId)
}

// A sender stated only as an address is named by the live contact that holds
// it now. An address a contact gave up names nobody.
func TestABareAddressIsNamedOnlyByTheContactThatHoldsItNow(t *testing.T) {
	e := Setup(t)
	filedUnder := e.SeedContact(t, "Cleo Filed", &e.Rep1)
	holder := e.SeedContact(t, "Hana Holder", &e.Rep1)
	former := e.SeedContact(t, "Fritz Former", &e.Rep1)
	held := logMailFrom(t, e, "hana@buyer.example", filedUnder)
	given := logMailFrom(t, e, "fritz.old@buyer.example", filedUnder)
	giveAddress(t, e, holder, "hana@buyer.example")
	giveAddress(t, e, former, "fritz.old@buyer.example")
	giveAddress(t, e, former, "fritz.new@buyer.example")

	if got := namedContact(listedAuthorOn(t, e, e.Rep1, filedUnder, held)); got != holder {
		t.Errorf("a mail from a held address names %v, want its holder %v", got, holder)
	}
	if got := namedContact(listedAuthorOn(t, e, e.Rep1, filedUnder, given)); !got.IsZero() {
		t.Errorf("a mail from an address its contact gave up names %v, want the address only", got)
	}
}

// A captured mail's From row names a contact this reader may not see. A
// visible contact then takes over the address. The stated contact stays the
// only candidate, so the reader sees the address and never the new holder.
func TestAStatedSenderOutOfSightIsNotReplacedByTheAddressesNewHolder(t *testing.T) {
	e := Setup(t)
	stated := e.SeedContact(t, "Sven Stated", &e.Rep1)
	mail := captureCommitmentMessage(t, e, stated, "stated-sender@buyer.example", "Offer",
		"Could you send the offer?", "inbound", time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC))
	address := commitmentMailAddress(t, e, stated)
	if named := captureSender(t, e, mail, address, "buyer.example"); named != stated {
		t.Fatalf("capture named %v as the sender, want %v", named, stated)
	}
	private := "owner"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](stated), contacts.UpdateContactInput{
		Visibility: &private,
		Emails:     []contacts.ContactEmailInput{{Email: "sven.new@buyer.example", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("hiding the sender and moving their address: %v", err)
	}
	newHolder := e.SeedContact(t, "Nora New Holder", &e.Rep2)
	giveAddress(t, e, newHolder, address)
	filedUnder := e.SeedContact(t, "Cleo Filed", &e.Rep2)
	LinkActivity(t, OwnerConn(t), mail, "contact", filedUnder)

	if got := namedContact(listedAuthorOn(t, e, e.Rep2, filedUnder, mail)); !got.IsZero() {
		t.Errorf("a reader who cannot see the stated sender is shown %v, want the address only, never the new holder %v", got, newHolder)
	}
}
