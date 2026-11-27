// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The audit row of an email or phone replace-set states both sides in the
// contract's row shape. The History tab reads them with one rule, and the
// contact.updated event carries the keys a subscriber was promised.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAReplacedSetRecordsItsRowsInTheShapeOfTheBeforeImage(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()

	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Ada Lovelace",
		Emails:   []ContactEmailInput{{Email: "ada@sets.test", EmailType: "work", IsPrimary: true}},
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if _, err := e.store.UpdateContact(ctx, id, UpdateContactInput{
		Emails: []ContactEmailInput{
			{Email: "ada@sets.test", EmailType: "work", IsPrimary: true},
			{Email: "ada@home.test", EmailType: "personal", Position: 1},
		},
		Phones: []ContactPhoneInput{{Phone: "+493011111111", PhoneType: "work", IsPrimary: true}},
		Source: "test",
	}); err != nil {
		t.Fatalf("replacing the sets: %v", err)
	}

	before, after := auditImagesHolding(ctx, t, e.store, entityContact, id.UUID, "emails")
	emails := imageRows(t, after, "emails", 2)
	wantImage(t, emails[1], "after emails[1]", "email", "ada@home.test")
	wantImage(t, emails[1], "after emails[1]", "email_type", "personal")
	wantImage(t, imageRows(t, before, "emails", 1)[0], "before emails[0]", "email", "ada@sets.test")
	wantImage(t, imageRows(t, after, "phones", 1)[0], "after phones[0]", "phone", "+493011111111")
	wantImage(t, before, "before", "phones", nil)

	// The forms send every set on every save; a set that did not move is no
	// change to record.
	held := countAuditRowsHolding(ctx, t, e.store, entityContact, id.UUID, "emails")
	title := "Countess"
	if _, err := e.store.UpdateContact(ctx, id, UpdateContactInput{
		Title:  &title,
		Emails: []ContactEmailInput{{Email: "ada@sets.test", EmailType: "work", IsPrimary: true}, {Email: "ada@home.test", EmailType: "personal", Position: 1}},
		Phones: []ContactPhoneInput{{Phone: "+493011111111", PhoneType: "work", IsPrimary: true}},
		Source: "test",
	}); err != nil {
		t.Fatalf("saving the title with the sets unchanged: %v", err)
	}
	if n := countAuditRowsHolding(ctx, t, e.store, entityContact, id.UUID, "emails"); n != held {
		t.Errorf("an unchanged email set was recorded again: %d rows hold emails, want %d", n, held)
	}
}

// imageRows reads one list out of an audit image, as the rows it holds.
//
//craft:ignore naked-any an audit image is jsonb: every row decodes to a map of any
func imageRows(t *testing.T, image map[string]any, key string, want int) []map[string]any {
	t.Helper()
	list, ok := image[key].([]any)
	if !ok || len(list) != want {
		t.Fatalf("%s = %v, want a list of %d row(s)", key, image[key], want)
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%s holds %v, want a row object", key, item)
		}
		rows = append(rows, row)
	}
	return rows
}
