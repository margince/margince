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
