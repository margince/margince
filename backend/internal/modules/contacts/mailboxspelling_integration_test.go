// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// One mailbox whose domain is spelled in Unicode and in punycode is one
// contact. uq_contact_email_dedupe compares stored text, so the writer asks
// for the other spelling itself.

import (
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
