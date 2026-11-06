// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An agent's card fills a blank and replaces no answer. A phone someone removed
// is a blank, and a live one is an answer.
func TestAnAgentsCardFillsAPhoneOnlyWhereNoLiveOneIsHeld(t *testing.T) {
	const held, onCard = "+493011111111", "+493022222222"
	for name, tc := range map[string]struct {
		archive bool
		want    []string
	}{
		"a live phone is kept":     {false, []string{held}},
		"an archived phone is not": {true, []string{onCard}},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			contact, err := e.store.CreateContact(e.as(), CreateContactInput{
				FullName: "Kim Kept", Source: "manual",
				Emails: []ContactEmailInput{{Email: "kim@kept.example", EmailType: emailTypeWork, IsPrimary: true}},
				Phones: []ContactPhoneInput{{Phone: held, PhoneType: "work", IsPrimary: true}},
			})
			if err != nil {
				t.Fatal(err)
			}
			contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
			if tc.archive {
				ctx := e.as()
				if err := e.store.tx(ctx, func(tx pgx.Tx) error {
					_, err := tx.Exec(ctx, `UPDATE contact_phone SET archived_at = now() WHERE contact_id = $1`, contactID)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			importCards(e.asUsersAgent(t), t, e, "BEGIN:VCARD\nFN:Kim Kept\nEMAIL;TYPE=WORK:kim@kept.example\nTEL;TYPE=CELL:"+onCard+"\nEND:VCARD\n")
			if live := livePhones(e.as(), t, e, contactID); !slices.Equal(live, tc.want) {
				t.Errorf("live phones are %v, want %v", live, tc.want)
			}
		})
	}
}

// An agent's card fills an empty address and leaves a held one, even one a card
// wrote, which a user's next card may still move.
func TestAnAgentsCardFillsAnAddressOnlyWhereNoneIsHeld(t *testing.T) {
	e := setupDedupe(t)
	const blankEmail, heldEmail = "blank.address@split.example.test", "held.address@split.example.test"
	blank := importCard(t, e, "BEGIN:VCARD\nFN:Bea Blank\nEMAIL:"+blankEmail+"\nEND:VCARD\n")
	importCards(e.asUsersAgent(t), t, e, cardWithAddress(blankEmail, "Filled Street 1", "Filled Town", "33333"))
	if got := contactAddressParts(t, e, *blank.ContactID); got[0] != "Filled Street 1" {
		t.Errorf("the agent's card left the empty address as %q", got)
	}

	held := importCard(t, e, cardWithAddress(heldEmail, "Held Street 1", "Held Town", "11111"))
	before := contactAddressParts(t, e, *held.ContactID)
	results := importCards(e.asUsersAgent(t), t, e, cardWithAddress(heldEmail, "Agent Street 2", "Agent Town", "22222"))
	if got := contactAddressParts(t, e, *held.ContactID); got != before {
		t.Errorf("the agent's card moved the address to %q, want %q", got, before)
	}
	if !slices.ContainsFunc(results[0].Fields, func(f VCardField) bool { return f.Field == fieldAddress && f.Action == VCardFieldKept }) {
		t.Errorf("the agent's report reads %+v, want the address kept", results[0].Fields)
	}
	importCard(t, e, cardWithAddress(heldEmail, "User Street 3", "User Town", "44444"))
	if got := contactAddressParts(t, e, *held.ContactID); got[0] != "User Street 3" {
		t.Errorf("the user's card left the address as %q", got)
	}
}
