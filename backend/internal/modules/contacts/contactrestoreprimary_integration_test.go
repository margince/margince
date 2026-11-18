// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A contact archived while it held a primary address and a primary number of
// each type comes back with one of each: the first by reading order. The index
// ignores archived rows, so the old shape survives in them.
func TestRestoringAContactWithOldPerTypePrimariesKeepsTheFirstOfEach(t *testing.T) {
	e := setupDedupe(t)
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	actor.Permissions.Objects[contactEntity] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	ctx := principal.WithActor(e.as(), actor)

	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Old Shape",
		Emails: []ContactEmailInput{
			{Email: "old@work.example", EmailType: "work", IsPrimary: true, Position: 1},
			{Email: "old@home.example", EmailType: "personal", Position: 0},
		},
		Phones: []ContactPhoneInput{
			{Phone: "+493011111111", PhoneType: "work", IsPrimary: true, Position: 1},
			{Phone: "+493022222222", PhoneType: "mobile", Position: 0},
		},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if _, err := e.store.ArchiveContact(ctx, id, nil); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE contact_email SET is_primary = true WHERE contact_id = $1`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE contact_phone SET is_primary = true WHERE contact_id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("writing the old shape into the archived rows: %v", err)
	}

	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := e.store.RestoreContactTx(ctx, tx, id, nil, pairRestore)
		return err
	}); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	restored, err := e.store.GetContact(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if primary := primaryEmails(liveEmailRows(restored)); len(primary) != 1 || primary[0] != "old@home.example" {
		t.Errorf("primary addresses = %v, want only the one at position 0", primary)
	}
	var primaries []string
	for _, p := range livePhoneRows(restored) {
		if p.IsPrimary {
			primaries = append(primaries, p.Phone)
		}
	}
	if len(primaries) != 1 || primaries[0] != "+493022222222" {
		t.Errorf("primary numbers = %v, want only the one at position 0", primaries)
	}
}
