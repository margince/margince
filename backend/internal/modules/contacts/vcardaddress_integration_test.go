// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func importCard(t *testing.T, e *dedupeEnv, card string) VCardResult {
	t.Helper()
	cards, err := ParseVCards(strings.NewReader(card))
	if err != nil {
		t.Fatal(err)
	}
	results, err := e.store.ImportVCards(e.as(), cards)
	if err != nil || len(results) != 1 || results[0].ContactID == nil {
		t.Fatalf("importing the card: %+v, %v", results, err)
	}
	return results[0]
}

func cardWithAddress(email, street, city, postal string) string {
	return "BEGIN:VCARD\nFN:Card Holder\nEMAIL:" + email + "\nADR;TYPE=WORK:;;" + street + ";" + city + ";;" + postal + ";Germany\nEND:VCARD\n"
}

func contactAddressParts(t *testing.T, e *dedupeEnv, id ids.ContactID) [6]string {
	t.Helper()
	got, err := e.store.GetContact(e.as(), id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	return addressPartsOf(addressColumns(got.Address))
}

func TestASecondCardMovesTheAddressTheFirstCardWrote(t *testing.T) {
	e := setupDedupe(t)
	const email = "moved.card@split.example.test"
	first := importCard(t, e, cardWithAddress(email, "Old Street 1", "Old Town", "11111"))
	second := importCard(t, e, cardWithAddress(email, "New Street 2", "New Town", "22222"))
	if second.Outcome != VCardUpdated || *second.ContactID != *first.ContactID {
		t.Fatalf("the second card = %+v, want an update of the first contact", second)
	}
	if got := contactAddressParts(t, e, *first.ContactID); got != [6]string{"New Street 2", "<nil>", "<nil>", "22222", "New Town", "DE"} {
		t.Errorf("address = %q, want the second card's parts", got)
	}
}

func TestACardFillsAnEmptyAddressAndLeavesATypedOne(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	typedLine := "Typed Road 1"
	create := func(name, email string, address *crmcontracts.Address) ids.ContactID {
		// The source a request may name is not the import's evidence.
		made, err := e.store.CreateContact(ctx, CreateContactInput{
			FullName: name, Source: vcardSource, Address: address,
			Emails: []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
		})
		if err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		return ids.From[ids.ContactKind](ids.UUID(made.Id))
	}
	empty := create("Empty Address", "empty.address@split.example.test", nil)
	typed := create("Typed Address", "typed.address@split.example.test", &crmcontracts.Address{Line1: &typedLine})
	edited := *importCard(t, e, cardWithAddress("edited.address@split.example.test", "Card Street 3", "Card Town", "33333")).ContactID
	if _, err := e.store.UpdateContact(ctx, edited, UpdateContactInput{Address: &crmcontracts.Address{Line1: &typedLine}}); err != nil {
		t.Fatalf("editing the address: %v", err)
	}

	importCard(t, e, cardWithAddress("empty.address@split.example.test", "Card Street 1", "Card Town", "11111"))
	importCard(t, e, cardWithAddress("typed.address@split.example.test", "Card Street 2", "Card Town", "22222"))
	importCard(t, e, cardWithAddress("edited.address@split.example.test", "Card Street 4", "Card Town", "44444"))

	if got := contactAddressParts(t, e, empty); got != [6]string{"Card Street 1", "<nil>", "<nil>", "11111", "Card Town", "DE"} {
		t.Errorf("empty address = %q, want the card's parts", got)
	}
	for name, id := range map[string]ids.ContactID{"typed on create": typed, "edited by hand": edited} {
		if got := contactAddressParts(t, e, id); got[0] != typedLine || got[4] != "<nil>" {
			t.Errorf("address %s = %q, want the typed line kept", name, got)
		}
	}
}
