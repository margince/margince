// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The row scope on the billing-contacts read, from both ends.
//
// This is the risk the feature carries. A billing contact is a relationship
// between two records the caller may hold different rights over, and the finance
// card renders it beside figures a wider audience is entitled to. Without the
// scope clause the panel would name contacts a reader cannot open anywhere else in
// the product — an edge becoming a way to enumerate a restricted roster.
//
// The fixture is deliberately TEAM-scoped. An unbounded admin short-circuits the
// scope clause entirely, so a test written as one would pass against a read that
// has no clause at all.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// billingReaderPerms is a bounded rep holding every grant the read asks for.
var billingReaderPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":      {Read: true},
		"company":      {Read: true},
		"relationship": {Read: true, Create: true},
	},
	RowScope: principal.RowScopeTeam,
}

// seedBillingEdge writes one billing_contact row straight through the owner
// connection, past the store's own gates — the point here is what the READ
// does with a row that already exists, not whether the write would allow it.
func seedBillingEdge(t *testing.T, contactID, companyID ids.UUID, role string) {
	t.Helper()
	if _, err := OwnerConn(t).Exec(context.Background(), `
		INSERT INTO relationship (kind, contact_id, company_id, role, source, captured_by)
		VALUES ('billing_contact', $1, $2, $3, 'test', 'human:x')`,
		contactID, companyID, role); err != nil {
		t.Fatalf("seeding a billing edge: %v", err)
	}
}

func TestBillingContactsHideAContactTheReaderMayNotSee(t *testing.T) {
	e := Setup(t)

	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	mine := e.SeedContact(t, "Visible Payer", &e.Rep1)
	// Capture-private to another team's rep, so the reader below cannot open
	// this contact anywhere in the product.
	theirs := e.SeedContact(t, "Hidden Payer", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", theirs, e.Rep3)

	seedBillingEdge(t, mine, company, "recipient")
	seedBillingEdge(t, theirs, company, "approver")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, billingReaderPerms)
	var got []string
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		rows, err := e.Contacts.BillingContactsFor(rep, tx, ids.From[ids.CompanyKind](company))
		if err != nil {
			return err
		}
		for _, r := range rows {
			got = append(got, r.FullName)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading billing contacts: %v", err)
	}
	if len(got) != 1 || got[0] != "Visible Payer" {
		t.Fatalf("billing contacts = %v, want exactly [Visible Payer] — the hidden contact leaked through the edge", got)
	}
}

func TestBillingRolesHideACompanyTheReaderMayNotSee(t *testing.T) {
	e := Setup(t)

	contact := e.SeedContact(t, "External Accountant", &e.Rep1)
	mine := e.SeedCompany(t, "Visible Customer", &e.Rep1)
	theirs := e.SeedCompany(t, "Hidden Customer", &e.Rep3)
	e.MakeCapturePrivate(t, "company", theirs, e.Rep3)

	seedBillingEdge(t, contact, mine, "recipient")
	seedBillingEdge(t, contact, theirs, "recipient")

	// The inverse leak, and the one that matters more: an accountant's page
	// listing every customer they bill for would disclose the client list of a
	// team this reader is not on.
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, billingReaderPerms)
	var got []string
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		rows, err := e.Contacts.BillingRolesOf(rep, tx, ids.From[ids.ContactKind](contact))
		if err != nil {
			return err
		}
		for _, r := range rows {
			got = append(got, r.CompanyName)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading billing roles: %v", err)
	}
	if len(got) != 1 || got[0] != "Visible Customer" {
		t.Fatalf("billing roles = %v, want exactly [Visible Customer] — the hidden company leaked through the edge", got)
	}
}
