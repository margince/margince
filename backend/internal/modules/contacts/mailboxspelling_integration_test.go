// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// One mailbox whose domain is spelled in Unicode and in punycode is one
// contact. uq_contact_email_dedupe compares stored text, so the writer asks
// for the other spelling itself.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheOtherSpellingOfAHeldMailboxIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, held, offered string }{
		{"punycode after Unicode", "anna@bücher.example", "anna@xn--bcher-kva.example"},
		{"Unicode after punycode", "anna@xn--bcher-kva.example", "ANNA@BÜCHER.EXAMPLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			first, err := e.store.CreateContact(ctx, CreateContactInput{
				FullName: "Anna Brandt",
				Emails:   []ContactEmailInput{{Email: tc.held, EmailType: "work", IsPrimary: true}},
				Source:   "test",
			})
			if err != nil {
				t.Fatalf("creating the first contact: %v", err)
			}

			_, err = e.store.CreateContact(ctx, CreateContactInput{
				FullName: "Anna B.",
				Emails:   []ContactEmailInput{{Email: tc.offered, EmailType: "work", IsPrimary: true}},
				Source:   "test",
			})
			var dup *DuplicateEmailError
			if !errors.As(err, &dup) {
				t.Fatalf("a second contact for %s took %s, err = %v; want a duplicate email refusal", tc.held, tc.offered, err)
			}

			second, err := e.store.CreateContact(ctx, CreateContactInput{
				FullName: "Somebody Else",
				Emails:   []ContactEmailInput{{Email: "other@xn--bcher-kva.example", EmailType: "work", IsPrimary: true}},
				Source:   "test",
			})
			if err != nil {
				t.Fatalf("another mailbox at the same domain was refused: %v", err)
			}
			_, err = e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(second.Id)), UpdateContactInput{
				Emails: []ContactEmailInput{{Email: tc.offered, EmailType: "work", IsPrimary: true}},
				Source: "test",
			})
			if !errors.As(err, &dup) || dup.ExistingID.UUID != ids.UUID(first.Id) {
				t.Fatalf("an update to %s while %s is held answered %v; want a refusal naming the holder", tc.offered, tc.held, err)
			}
		})
	}
}

// Mail from the other spelling files onto the contact that holds the mailbox
// rather than minting a second one.
func TestMailFromTheOtherSpellingLandsOnTheSameContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "anna@xn--bcher-kva.example", "Anna Brandt", "xn--bcher-kva.example"))
	if err != nil {
		t.Fatalf("ensure from the punycode spelling: %v", err)
	}
	second, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "anna@bücher.example", "Anna Brandt", "xn--bcher-kva.example"))
	if err != nil {
		t.Fatalf("ensure from the Unicode spelling: %v", err)
	}
	if second.ContactID != first.ContactID {
		t.Errorf("the Unicode spelling landed on %s, the punycode one on %s; want one contact", second.ContactID, first.ContactID)
	}
}

// An address erased under one spelling of its domain stays erased under the
// other, so the punycode twin cannot bring the person back.
func TestAnAddressErasedInUnicodeStaysDeadInPunycode(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO erasure_suppression (kind, value_hash) VALUES ('email', $1)`,
			storekit.SuppressionHash("anna@bücher.example"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "anna@xn--bcher-kva.example", "Anna Brandt", "xn--bcher-kva.example"))
	if !errors.Is(err, ErrCounterpartySuppressed) {
		t.Fatalf("the punycode twin of an erased address = %v, want ErrCounterpartySuppressed", err)
	}
}

// legacySpelling moves a record's address to a spelling the writers now refuse
// beside its twin, the way records stood before both spellings were one key.
func legacySpelling(ctx context.Context, t *testing.T, e *dedupeEnv, statement string, id ids.UUID, address string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, statement, id, address)
		return err
	}); err != nil {
		t.Fatalf("storing the legacy spelling: %v", err)
	}
}

// Where each spelling already has its own contact, mail to a spelling lands on
// the contact holding that spelling, not on whichever is older.
func TestMailToEachSpellingLandsOnItsOwnContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const unicode, puny = "anna@bücher.example", "anna@xn--bcher-kva.example"
	older, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Anna Brandt",
		Emails:   []ContactEmailInput{{Email: unicode, EmailType: "work", IsPrimary: true}},
		Source:   "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Anna B.",
		Emails:   []ContactEmailInput{{Email: "placeholder@elsewhere.example", EmailType: "work", IsPrimary: true}},
		Source:   "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	legacySpelling(ctx, t, e, `UPDATE contact_email SET email = $2 WHERE contact_id = $1`, ids.UUID(newer.Id), puny)

	for address, want := range map[string]ids.UUID{unicode: ids.UUID(older.Id), puny: ids.UUID(newer.Id)} {
		got, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, address, "Anna Brandt", "xn--bcher-kva.example"))
		if err != nil {
			t.Fatalf("ensure %s: %v", address, err)
		}
		if got.ContactID.UUID != want {
			t.Errorf("mail from %s landed on %s, want the contact holding that spelling, %s", address, got.ContactID, want)
		}
		booked, err := e.store.EnsureContactByEmail(ctx, "Anna", address, "booking", storekit.AdmitSelfBooking)
		if err != nil || booked != want {
			t.Errorf("a booking from %s reused %s (err %v), want %s", address, booked, err, want)
		}
	}
}

// The same for leads: a door files on the lead holding the spelling it saw.
func TestALeadHoldingTheTypedSpellingIsTheOneFiledOn(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const unicode, puny = "jonas@bücher.example", "jonas@xn--bcher-kva.example"
	older := e.createLead(ctx, t, "Jonas Petersen", unicode, "Bücher")
	newer := e.createLead(ctx, t, "Jonas P.", "placeholder@elsewhere.example", "Bücher")
	legacySpelling(ctx, t, e, `UPDATE lead SET email = $2 WHERE id = $1`, newer.UUID, puny)

	for address, want := range map[string]ids.LeadID{unicode: older, puny: newer} {
		got, err := e.store.CounterpartyLead(ctx, address)
		if err != nil {
			t.Fatalf("counterparty lead for %s: %v", address, err)
		}
		if got != want {
			t.Errorf("mail from %s files on lead %s, want %s", address, got, want)
		}
	}
}

// A booking from the other spelling reuses the contact instead of failing on
// a duplicate address.
func TestABookingFromTheOtherSpellingReusesTheContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	held, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Anna Brandt",
		Emails:   []ContactEmailInput{{Email: "anna@xn--bcher-kva.example", EmailType: "work", IsPrimary: true}},
		Source:   "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	booked, err := e.store.EnsureContactByEmail(ctx, "Anna", "anna@bücher.example", "booking", storekit.AdmitSelfBooking)
	if err != nil {
		t.Fatalf("booking from the Unicode spelling: %v", err)
	}
	if booked != ids.UUID(held.Id) {
		t.Errorf("the booking made %s, want the contact that holds the mailbox, %s", booked, held.Id)
	}
}
