// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// An address whose local part names a sending system (noreply@, receipts@)
// becomes no contact on any path.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const sendingSystemAddress = "signing-noreply@machine.test"

func requireMachineAddressFault(t *testing.T, err error) {
	t.Helper()
	var fault *values.ParseError
	if !errors.As(err, &fault) || fault.Field != "emails" || fault.Code != MachineAddressCode {
		t.Fatalf("err = %v, want an emails fault %q", err, MachineAddressCode)
	}
}

// writeShapeRows counts the three tables every mutation writes together.
func writeShapeRows(ctx context.Context, t *testing.T, e *dedupeEnv) [3]int {
	t.Helper()
	var n [3]int
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM contact), (SELECT count(*) FROM audit_log), (SELECT count(*) FROM event_outbox)`).
			Scan(&n[0], &n[1], &n[2])
	})
	if err != nil {
		t.Fatalf("counting write-shape rows: %v", err)
	}
	return n
}

func liveAddressesOf(ctx context.Context, t *testing.T, e *dedupeEnv, id ids.UUID) []string {
	t.Helper()
	var held []string
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT coalesce(array_agg(email ORDER BY email), '{}') FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL`, id).Scan(&held)
	})
	if err != nil {
		t.Fatalf("reading addresses: %v", err)
	}
	return held
}

// legacyMachineContact is a contact whose address was stored before the rule
// existed: created through the real writer, then given the address in place.
func legacyMachineContact(ctx context.Context, t *testing.T, e *dedupeEnv, address string) ids.UUID {
	t.Helper()
	made, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Legacy Sender", Source: "manual",
		Emails: []ContactEmailInput{{Email: "legacy-" + ids.NewV7().String() + "@machine.test", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the legacy contact: %v", err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE contact_email SET email = $2 WHERE contact_id = $1`, made.Id, address)
		return err
	}); err != nil {
		t.Fatalf("giving the legacy contact its machine address: %v", err)
	}
	return ids.UUID(made.Id)
}

func TestACreateWithASendingSystemsAddressIsRefusedAndWritesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	before := writeShapeRows(ctx, t, e)

	_, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Signing-Noreply", Source: "manual",
		Emails: []ContactEmailInput{{Email: sendingSystemAddress, EmailType: emailTypeWork, IsPrimary: true}},
	})
	requireMachineAddressFault(t, err)
	if after := writeShapeRows(ctx, t, e); after != before {
		t.Errorf("contact, audit_log, event_outbox rows went from %v to %v on a refused create", before, after)
	}
}

func TestAnUpdateCannotReplaceAnAddressWithASendingSystemsAddress(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	made, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Anna Weber", Source: "manual",
		Emails: []ContactEmailInput{{Email: "anna@machine.test", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(made.Id)), UpdateContactInput{
		Emails: []ContactEmailInput{{Email: "receipts@machine.test", EmailType: emailTypeWork, IsPrimary: true}},
	})
	requireMachineAddressFault(t, err)
	if held := liveAddressesOf(ctx, t, e, ids.UUID(made.Id)); !slices.Equal(held, []string{"anna@machine.test"}) {
		t.Errorf("addresses after the refused update = %v, want the old one kept", held)
	}
}

// The public booking reaches the store through EnsureContactByEmail, which
// hands back a contact already holding the address. A contact stored before
// the rule must not take the booking either.
func TestEnsureContactByEmailRefusesASendingSystemsAddressEvenWhenAContactHoldsIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	legacyMachineContact(ctx, t, e, sendingSystemAddress)

	_, err := e.store.EnsureContactByEmail(ctx, "Signing", sendingSystemAddress, "public_booking")
	requireMachineAddressFault(t, err)
	if _, err := e.store.EnsureContactByEmail(ctx, "Notifications", "notifications@machine.test", "public_booking"); err == nil {
		t.Error("a booking from notifications@ minted a contact")
	}
}

// Capture's counterparty path reaches the child-row writer without the parse
// step, so the writer asks the rule itself.
func TestTheCounterpartyPathRefusesASendingSystemsAddress(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	before := writeShapeRows(ctx, t, e)

	_, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "mailer-daemon@counterparty.test", "Mail Delivery", "counterparty.test"))
	requireMachineAddressFault(t, err)
	if after := writeShapeRows(ctx, t, e); after[0] != before[0] {
		t.Errorf("contacts went from %d to %d on a refused counterparty", before[0], after[0])
	}
}

func TestAVCardWithASendingSystemsAddressIsSkippedAndTheRestOfTheFileLands(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:First Human\nEMAIL:first@vcard-machine.test\nEND:VCARD\n"+
			"BEGIN:VCARD\nFN:Signing Service\nEMAIL:"+sendingSystemAddress+"\nEND:VCARD\n"+
			"BEGIN:VCARD\nFN:Third Human\nEMAIL:third@vcard-machine.test\nEND:VCARD\n")

	want := []VCardOutcome{VCardCreated, VCardSkipped, VCardCreated}
	for i, result := range results {
		if result.Outcome != want[i] {
			t.Errorf("card %d outcome = %q, want %q", i, result.Outcome, want[i])
		}
	}
	if len(results) != 3 || results[1].Reason != vcardMachineAddress {
		t.Fatalf("results = %+v, want the middle card skipped with its reason", results)
	}
	if got := countContactsNamed(ctx, t, e, "Signing Service"); got != 0 {
		t.Errorf("contacts named Signing Service = %d, want 0", got)
	}
}

// A card that matches an existing contact takes the update arm, which never
// reaches the create's check.
func TestAVCardMatchingAContactCannotCarryASendingSystemsAddressOntoIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	made, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Dana Match", Source: "manual",
		Emails: []ContactEmailInput{{Email: "dana@vcard-match.test", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Dana Match\nEMAIL:dana@vcard-match.test\nEMAIL:notifications@vcard-match.test\nEND:VCARD\n")
	if len(results) != 1 || results[0].Outcome != VCardSkipped {
		t.Fatalf("results = %+v, want the card skipped", results)
	}
	if held := liveAddressesOf(ctx, t, e, ids.UUID(made.Id)); !slices.Equal(held, []string{"dana@vcard-match.test"}) {
		t.Errorf("addresses after the card = %v, want only the contact's own", held)
	}
}

func TestABoughtSendingSystemsAddressFillsNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	made, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "No Address Yet", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	var landed filled
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		landed, err = fillEmail(ctx, tx, ids.UUID(made.Id), "provider", applicableClaims{email: "noreply@bought.test"})
		return err
	}); err != nil {
		t.Fatalf("fill: %v", err)
	}
	if landed.ok {
		t.Error("a bought noreply address was filled onto the contact")
	}
	if held := liveAddressesOf(ctx, t, e, ids.UUID(made.Id)); len(held) != 0 {
		t.Errorf("addresses = %v, want none", held)
	}
}

// A display name the writer chose stays, even where it is not the name parts.
func TestACreateAndAnUpdateKeepAChosenFullNameBesideItsParts(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, last := "Robert", "Fischer"
	made, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Bobby", FirstName: &first, LastName: &last, Source: "manual"})
	if err != nil || made.FullName != "Bobby" {
		t.Fatalf("create = %q, %v; want the chosen name Bobby stored", made.FullName, err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(made.Id))
	chosen, newLast := "Bob F.", "Fischer-Lang"
	updated, err := e.store.UpdateContact(ctx, id, UpdateContactInput{FullName: &chosen, FirstName: &first, LastName: &newLast})
	if err != nil || updated.FullName != "Bob F." {
		t.Fatalf("update = %q, %v; want the chosen name Bob F. stored", updated.FullName, err)
	}

	updated, err = e.store.UpdateContact(ctx, id, UpdateContactInput{LastName: &last})
	if err != nil {
		t.Fatalf("an update of the last name alone: %v", err)
	}
	if updated.FullName != "Bob F." {
		t.Errorf("full name after a last-name edit = %q, want it unchanged", updated.FullName)
	}
}

func TestMachineAddressContactsPagesEveryContactAndNamesTheMachineOnes(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	machine := legacyMachineContact(ctx, t, e, "receipts@page.test")
	anna, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Anna Notify Weber", Source: "manual",
		Emails: []ContactEmailInput{{Email: "anna.notify.weber@page.test", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(ctx, "system:test")

	var scanned, flagged []ids.UUID
	after := ids.Nil
	for {
		var page MachineAddressPage
		if err := e.store.tx(system, func(tx pgx.Tx) error {
			var err error
			page, err = MachineAddressContactsTx(system, tx, after, 1)
			return err
		}); err != nil {
			t.Fatalf("paging: %v", err)
		}
		if len(page.Scanned) == 0 {
			break
		}
		scanned = append(scanned, page.Scanned...)
		flagged = append(flagged, page.Machine...)
		after = page.Scanned[len(page.Scanned)-1]
	}
	if !slices.Contains(scanned, ids.UUID(anna.Id)) || !slices.Contains(scanned, machine) {
		t.Errorf("scanned = %v, want both contacts read one page at a time", scanned)
	}
	if !slices.Equal(flagged, []ids.UUID{machine}) {
		t.Errorf("flagged = %v, want only the receipts@ contact", flagged)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := MachineAddressContactsTx(ctx, tx, ids.Nil, 10)
		return err
	}); err == nil {
		t.Error("a human caller read every contact's addresses")
	}
}
