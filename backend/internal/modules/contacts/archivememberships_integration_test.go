// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Archiving a company or a contact removes its Shortlist memberships and logs each
// removal on the list, and leaves another record's membership in place.
func TestArchivingACompanyOrContactRemovesOnlyItsShortlistMemberships(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, companyID := e.seedEmployedContact(ctx, t,
		"Nora Lindqvist", "nora@fjordtek.test", "Fjordtek AB", "fjordtek.test")
	bystander := e.seedAcmeCompany(t)

	var companyList, contactList string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`INSERT INTO list (name, entity_type) VALUES ('Nordics', 'company') RETURNING id::text`).Scan(&companyList); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO list (name, entity_type) VALUES ('Champions', 'contact') RETURNING id::text`).Scan(&contactList); err != nil {
			return err
		}
		for _, member := range []struct {
			list, kind string
			id         ids.UUID
		}{
			{companyList, "company", companyID.UUID},
			{companyList, "company", bystander.UUID},
			{contactList, "contact", contactID.UUID},
		} {
			if _, err := tx.Exec(ctx,
				`INSERT INTO list_member (list_id, entity_type, entity_id, added_by) VALUES ($1, $2, $3, 'human:test')`,
				member.list, member.kind, member.id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("shortlisting the records: %v", err)
	}

	archiver := e.asArchiver()
	if _, err := e.store.ArchiveContact(archiver, contactID, nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
	if _, err := e.store.ArchiveCompany(archiver, companyID, nil); err != nil {
		t.Fatalf("archiving the company: %v", err)
	}

	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := e.store.tx(ctx, func(tx pgx.Tx) error { return tx.QueryRow(ctx, query, args...).Scan(&n) }); err != nil {
			t.Fatalf("counting: %v", err)
		}
		return n
	}
	for _, record := range []struct {
		kind string
		id   ids.UUID
	}{{"company", companyID.UUID}, {"contact", contactID.UUID}} {
		if n := count(`SELECT count(*) FROM list_member WHERE entity_type = $1 AND entity_id = $2`, record.kind, record.id); n != 0 {
			t.Errorf("the archived %s is still on %d Shortlist(s)", record.kind, n)
		}
		if n := count(`SELECT count(*) FROM list_member_event
			WHERE entity_type = $1 AND entity_id = $2 AND action = 'removed' AND reason = 'record_archived'`,
			record.kind, record.id); n != 1 {
			t.Errorf("the list logged %d archive removals for the %s, want 1", n, record.kind)
		}
	}
	if n := count(`SELECT count(*) FROM list_member WHERE entity_type = 'company' AND entity_id = $1`, bystander.UUID); n != 1 {
		t.Errorf("another company's membership count is %d, want it untouched at 1", n)
	}
}
