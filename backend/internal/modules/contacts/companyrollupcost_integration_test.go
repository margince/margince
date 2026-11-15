// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompanyPipelineFieldsShareOneRollupRead(t *testing.T) {
	e := setupDedupe(t)
	frozen := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	original := rollupClock
	t.Cleanup(func() { rollupClock = original })
	rollupClock = func() time.Time { return frozen }
	e.store.SetClock(rollupClock)
	companies := seedRollupCountCompanies(t, e)

	for _, route := range []string{"single", "borrowed-transaction", "list"} {
		for contactGrants := range 4 {
			t.Run(fmt.Sprintf("%s/contact-grants-%d", route, contactGrants), func(t *testing.T) {
				read := func(pipelineGrants int) ([]crmcontracts.Company, int64) {
					t.Helper()
					ctx := companyRollupReader(e, contactGrants, pipelineGrants)
					ctx, meter := database.WithStatementMeter(ctx)
					rows, err := readRollupCountCompanies(ctx, e.store, route, companies)
					if err != nil {
						t.Fatal(err)
					}
					cost := meter.Cost()
					if cost.Statements == 0 || cost.Failed != 0 {
						t.Fatalf("unobserved or failed company read: %+v", cost)
					}
					return rows, cost.Statements
				}
				_, baseline := read(0)
				for pipelineGrants := range 4 {
					rows, statements := read(pipelineGrants)
					wantExtra := int64(0)
					if pipelineGrants == 3 {
						wantExtra = 1
					}
					if statements != baseline+wantExtra {
						t.Fatalf("pipeline grants %d: %d statements, want %d + %d", pipelineGrants, statements, baseline, wantExtra)
					}
					wantRows := 1
					if route == "list" {
						wantRows = len(companies)
					}
					if len(rows) != wantRows {
						t.Fatalf("got %d companies, want %d", len(rows), wantRows)
					}
					for _, company := range rows {
						wantContacts := 0
						if ids.UUID(company.Id) == companies[0] {
							wantContacts = 1
						}
						assertRollupCountFields(t, company, contactGrants == 3, pipelineGrants == 3, route != "list", wantContacts)
					}
				}
			})
		}
	}
}

func seedRollupCountCompanies(t *testing.T, e *dedupeEnv) []ids.UUID {
	t.Helper()
	var companies []ids.UUID
	for _, name := range []string{"With an employee", "Without employees"} {
		company, err := e.store.CreateCompany(e.as(), CreateCompanyInput{DisplayName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		companies = append(companies, ids.UUID(company.Id))
	}
	contact, err := e.store.CreateContact(e.as(), CreateContactInput{FullName: "Company employee", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	companyID := ids.From[ids.CompanyKind](companies[0])
	if _, err := e.store.CreateRelationship(e.as(), CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
		IsCurrentPrimary: new(true), Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	return companies
}

func companyRollupReader(e *dedupeEnv, contactGrants, pipelineGrants int) context.Context {
	return principal.WithActor(e.as(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{
				"company":        {Read: true},
				"contact":        {Read: contactGrants&1 != 0},
				"relationship":   {Read: contactGrants&2 != 0},
				"deal":           {Read: pipelineGrants&1 != 0},
				"computed_field": {Read: pipelineGrants&2 != 0},
			},
		},
	})
}

func readRollupCountCompanies(ctx context.Context, store *Store, route string, companies []ids.UUID) ([]crmcontracts.Company, error) {
	companyID := ids.From[ids.CompanyKind](companies[0])
	if route == "list" {
		rows, _, err := store.ListCompanies(ctx, ListCompaniesInput{IDs: companies})
		return rows, err
	}
	var company crmcontracts.Company
	var err error
	if route == "borrowed-transaction" {
		err = store.tx(ctx, func(tx pgx.Tx) error {
			company, err = store.GetCompanyTx(ctx, tx, companyID, storekit.LiveOnly, CustomColumns{})
			return err
		})
	} else {
		company, err = store.GetCompany(ctx, companyID, storekit.LiveOnly)
	}
	return []crmcontracts.Company{company}, err
}

func assertRollupCountFields(t *testing.T, company crmcontracts.Company, contactsVisible, pipelineVisible, detail bool, wantContacts int) {
	t.Helper()
	if contactsVisible {
		if company.ContactCount == nil || *company.ContactCount != wantContacts {
			t.Fatalf("contact_count = %v, want %d", company.ContactCount, wantContacts)
		}
	} else if company.ContactCount != nil {
		t.Fatalf("contact_count must be absent without both grants: %v", company.ContactCount)
	}
	if !pipelineVisible {
		if company.OpenDealCount != nil || company.ComputedFields != nil {
			t.Fatalf("pipeline fields must be absent without both grants: %+v", company)
		}
		return
	}
	if company.OpenDealCount == nil || *company.OpenDealCount != 0 {
		t.Fatalf("an empty pipeline must count zero: %v", company.OpenDealCount)
	}
	if !detail {
		if company.ComputedFields != nil {
			t.Fatal("a company list must not acquire computed fields")
		}
		return
	}
	if company.ComputedFields == nil || len(*company.ComputedFields) != 5 {
		t.Fatalf("computed_fields = %v, want five fields", company.ComputedFields)
	}
	for _, field := range *company.ComputedFields {
		if field.Key == "open_pipeline" {
			if !field.Computable || field.ValueMinor == nil || *field.ValueMinor != 0 || field.Reason != nil {
				t.Fatalf("an empty pipeline must have a computable zero: %+v", field)
			}
			return
		}
	}
	t.Fatal("computed fields lack open_pipeline")
}
