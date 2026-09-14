// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The offline demo directory against the REAL database, because the one thing
// worth proving here is a property of the connection rather than of the SQL
// text: every read opens through the seam, which REFUSES a caller that has not
// said which workspace it is in rather than answering an empty mailbox.
//
// The first version of this directory used the bare pool. Every sync completed
// cleanly, the generator was handed a mailbox with zero accounts, and no error
// was logged anywhere. A unit test with a fake pool would have agreed with the
// mistake.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TestTheDirectoryReadsThroughTheSeam is the regression this file exists for.
func TestTheDirectoryReadsThroughTheSeam(t *testing.T) {
	e := integration.Setup(t)
	// The workspace must be BOUND, exactly as the capture job binds it before
	// calling the connector. Without it the read is refused rather than empty:
	// the failure is loud.
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	seat := ids.UUID(e.Rep1)

	company := seedOfflineDemoAccount(t, e, seat, "acme")

	box, err := offlineDemoDirectory{pool: e.Pool}.Mailbox(ctx, seat.String())
	if err != nil {
		t.Fatalf("reading the mailbox: %v", err)
	}
	if box.Email == "" {
		t.Error("the mailbox has no address, so no message can be written from it")
	}
	if len(box.Accounts) == 0 {
		t.Fatal("the directory returned no accounts — the seeded seat owns one")
	}
	var found bool
	for _, a := range box.Accounts {
		if a.CompanyID == company.String() {
			found = true
			if len(a.Contacts) == 0 {
				t.Error("the account carries no contacts, so the generator writes to nobody")
			}
			if a.Now.IsZero() {
				t.Error("the account carries no clock, so every message would be refused as undated")
			}
		}
	}
	if !found {
		t.Errorf("the seat owns %s and the directory did not return it", company)
	}
}

// TestTheDirectoryOnlyReturnsThisSeatsAccounts — a rep's inbox holds the
// accounts that rep owns. Returning another seat's would put correspondence in
// the wrong mailbox, and the sink would refuse the link anyway.
func TestTheDirectoryOnlyReturnsThisSeatsAccounts(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)

	mine := seedOfflineDemoAccount(t, e, ids.UUID(e.Rep1), "mine")
	theirs := seedOfflineDemoAccount(t, e, ids.UUID(e.Rep3), "theirs")

	box, err := offlineDemoDirectory{pool: e.Pool}.Mailbox(ctx, ids.UUID(e.Rep1).String())
	if err != nil {
		t.Fatalf("reading the mailbox: %v", err)
	}
	for _, a := range box.Accounts {
		if a.CompanyID == theirs.String() {
			t.Errorf("the mailbox for one seat returned %s, which another seat owns", theirs)
		}
	}
	var sawMine bool
	for _, a := range box.Accounts {
		if a.CompanyID == mine.String() {
			sawMine = true
		}
	}
	if !sawMine {
		t.Error("the seat's own account is missing from its mailbox")
	}
}

// seedOfflineDemoAccount writes one owned company with a contactable contact,
// which is the minimum the generator needs to write a thread.
func seedOfflineDemoAccount(t *testing.T, e *integration.Env, owner ids.UUID, slug string) ids.UUID {
	t.Helper()
	ctx := context.Background()
	var companyID ids.UUID
	err := e.Pool.QueryRow(ctx, `
		INSERT INTO company (display_name, lifecycle, owner_id, source, captured_by)
		VALUES ($2, 'customer', $1, 'test', 'human:test')
		RETURNING id`, owner, slug+" GmbH").Scan(&companyID)
	if err != nil {
		t.Fatalf("seeding a company: %v", err)
	}
	var contactID ids.UUID
	err = e.Pool.QueryRow(ctx, `
		INSERT INTO contact (full_name, source, captured_by)
		VALUES ($1, 'test', 'human:test') RETURNING id`, "Petra "+slug).Scan(&contactID)
	if err != nil {
		t.Fatalf("seeding a contact: %v", err)
	}
	if _, err := e.Pool.Exec(ctx, `
		INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, $2, true, 'test', 'human:test')`, contactID, "petra@"+slug+".example"); err != nil {
		t.Fatalf("seeding an address: %v", err)
	}
	if _, err := e.Pool.Exec(ctx, `
		INSERT INTO relationship (kind, contact_id, company_id, role, source, captured_by)
		VALUES ('employment', $1, $2, 'Head of IT', 'test', 'human:test')`, contactID, companyID); err != nil {
		t.Fatalf("seeding an employment: %v", err)
	}
	return companyID
}
