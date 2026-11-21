// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// A bought employment claim names its employer's domain as the provider spelt
// it. The account page shows the bought title for a company on the same
// domain, whichever of the two spellings the claim or the company holds.
func TestABoughtTitleReachesTheAccountWhenTheDomainIsSpeltOtherwise(t *testing.T) {
	for _, tc := range []struct{ name, companyDomain, claimDomain string }{
		{"claim in Unicode, company in punycode", "xn--bcher-kva.example", "Bücher.example"},
		{"claim in punycode, company in Unicode", "bücher.example", "xn--bcher-kva.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
				DisplayName: "Bücher Verlag", Source: "manual",
				Domains: []contacts.CompanyDomainInput{{Domain: tc.companyDomain, IsPrimary: true}},
			})
			if err != nil {
				t.Fatal(err)
			}
			contact := ids.NewV7()
			execAsOwner(t, e, `INSERT INTO contact (id, full_name, source, captured_by)
				VALUES ($1, 'Anna Brandt', 'manual', 'human:x')`, contact)
			companyID, contactID := companyIDOf(ids.UUID(company.Id)), ContactIDOf(contact)
			if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
				Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
			}); err != nil {
				t.Fatal(err)
			}
			seedOrphanClaim(t, e, contact, string(provider.ClaimCurrentEmployment),
				`{"company_domain":"`+tc.claimDomain+`","job_title":"Head of Revenue"}`)

			page, err := companySurfaceService(e).Assemble(e.Admin(), companyID)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range page.Contacts.Data {
				if ids.UUID(c.ContactId) == contact && c.ProviderTitle != nil && *c.ProviderTitle == "Head of Revenue" {
					return
				}
			}
			t.Errorf("the account page shows no bought title for the contact: %+v", page.Contacts.Data)
		})
	}
}
