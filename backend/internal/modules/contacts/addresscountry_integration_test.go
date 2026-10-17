// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Create and edit, on a contact and on a company, hold one country rule: a name
// is refused and a code lands upper-case.
func TestAnAddressCountryIsHeldToAlpha2OnEveryContactAndCompanyWrite(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.as(e.owner, principal.RowScopeAll)
	var invalid *InvalidCountryError

	if _, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Lan Pham", Source: "manual", Address: countryOf("Vietnam"),
	}); !errors.As(err, &invalid) {
		t.Fatalf("contact create with a country name: err = %v, want an invalid-country refusal", err)
	}
	if _, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Pham Trading", Source: "manual", Address: countryOf("DEU"),
	}); !errors.As(err, &invalid) {
		t.Fatalf("company create with an alpha-3 code: err = %v, want an invalid-country refusal", err)
	}

	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Lan Pham", Source: "manual", Address: countryOf("vn"),
	})
	if err != nil {
		t.Fatalf("contact create with a lower-case code: %v", err)
	}
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Pham Trading", Source: "manual", Address: countryOf("de"),
	})
	if err != nil {
		t.Fatalf("company create with a lower-case code: %v", err)
	}
	if contact.Address == nil || *contact.Address.Country != "VN" || company.Address == nil || *company.Address.Country != "DE" {
		t.Fatalf("stored countries = %v / %v, want VN / DE", contact.Address, company.Address)
	}

	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if _, err := e.store.UpdateContact(ctx, contactID, UpdateContactInput{Address: countryOf("zz")}); !errors.As(err, &invalid) {
		t.Fatalf("contact edit with an unassigned code: err = %v, want an invalid-country refusal", err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{Address: countryOf("")}); !errors.As(err, &invalid) {
		t.Fatalf("company edit with an empty country: err = %v, want an invalid-country refusal", err)
	}

	edited, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{Address: countryOf("fr")})
	if err != nil || *edited.Address.Country != "FR" {
		t.Fatalf("company edit with a lower-case code: %v / %v, want FR", edited.Address, err)
	}
}
