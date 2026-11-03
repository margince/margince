// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A company's main number and general address are written on create, edited
// and cleared in place, and filled by a site read only while empty. Neither is
// ever a contact.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACompanysPhoneAndEmailRoundTripThroughCreateAndUpdate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	phone, email := "+49 30 1234567", "Info@Switchboard.example"
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Switchboard GmbH", Source: "manual", Phone: &phone, Email: &email,
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	reread, err := e.store.GetCompany(ctx, companyID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Phone == nil || *reread.Phone != "+49301234567" || reread.Email == nil || string(*reread.Email) != "info@switchboard.example" {
		t.Fatalf("re-read phone %v email %v, want the parsed spellings", reread.Phone, reread.Email)
	}

	newPhone, newEmail := "+49 30 7654321", "hello@switchboard.example"
	before := e.companyAuditRows(ctx, t, companyID)
	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{Phone: &newPhone, Email: &newEmail}); err != nil {
		t.Fatalf("editing both: %v", err)
	}
	if got := e.companyAuditRows(ctx, t, companyID) - before; got != 1 {
		t.Errorf("editing both wrote %d audit rows, want one", got)
	}
	edited, err := e.store.GetCompany(ctx, companyID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Phone == nil || *edited.Phone != "+49307654321" || edited.Email == nil || string(*edited.Email) != newEmail {
		t.Fatalf("edited phone %v email %v", edited.Phone, edited.Email)
	}

	empty := ""
	cleared, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{Phone: &empty, Email: &empty})
	if err != nil {
		t.Fatalf("clearing both: %v", err)
	}
	if cleared.Phone != nil || cleared.Email != nil {
		t.Errorf("cleared phone %v email %v, want both absent", cleared.Phone, cleared.Email)
	}

	if contacts := e.contactsHolding(ctx, t, newEmail); contacts != 0 {
		t.Errorf("the company's address made %d contacts, want none", contacts)
	}
}

// A site read fills an empty number, and leaves a typed address as it is.
func TestASiteReadFillsAnEmptyCompanyPhoneAndLeavesATypedEmail(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	typed := "sales@typed.example"
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Typed GmbH", Source: "manual", Email: &typed})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	fact := func(field, value string) DeepReadFact {
		return DeepReadFact{
			Category: factCategoryCompany, Field: field, Value: value,
			EvidenceSnippet: value, SourceURL: "https://typed.example/impressum", Confidence: 0.9,
		}
	}
	read := ids.NewV7()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO site_read (id, company_id, target_kind, status, requested_by, seed_url)
			VALUES ($1, $2, 'company', 'done', 'human:test', 'https://typed.example/')`, read, companyID)
		return err
	}); err != nil {
		t.Fatalf("seeding the site read the facts came from: %v", err)
	}
	if err := e.store.ApplyDeepRead(ctx, DeepReadProposal{
		CompanyID: companyID, SourceURL: "https://typed.example/", SiteReadID: read,
		Facts: []DeepReadFact{fact(FactPhone, "+49 89 5550100"), fact(FactContactEmail, "info@typed.example")},
	}); err != nil {
		t.Fatalf("ApplyDeepRead: %v", err)
	}
	filled, err := e.store.GetCompany(ctx, companyID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if filled.Phone == nil || *filled.Phone != "+49895550100" {
		t.Errorf("phone = %v, want the site's number filled into the empty column", filled.Phone)
	}
	if filled.Email == nil || string(*filled.Email) != typed {
		t.Errorf("email = %v, want the typed %q left alone", filled.Email, typed)
	}
}

func (e *dedupeEnv) companyAuditRows(ctx context.Context, t *testing.T, company ids.CompanyID) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = $1`, company).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *dedupeEnv) contactsHolding(ctx context.Context, t *testing.T, address string) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM contact_email WHERE lower(email) = lower($1)`, address).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}
