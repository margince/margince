// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The batch half answers about a record not written yet what the ladder
// answers once it is. Each case writes the held record through the store and
// asks both.
func TestTheLadderAndItsBatchHalfAgree(t *testing.T) {
	street := func(line string) *crmcontracts.Address {
		return &crmcontracts.Address{Line1: &line, PostalCode: new("80331"), City: new("München")}
	}
	for name, tc := range map[string]struct {
		held, asked NewCompany
		pairs       bool
	}{
		"one brand": {held: NewCompany{DisplayName: "Fabrikam GmbH"}, asked: NewCompany{DisplayName: "Fabrikam"}, pairs: true},
		"similar names, nothing shared": {
			held: NewCompany{DisplayName: brandName}, asked: NewCompany{DisplayName: lineName},
		},
		"similar names at one street": {
			held:  NewCompany{DisplayName: brandName, Address: street("Marienplatz 8")},
			asked: NewCompany{DisplayName: lineName, Address: street("marienplatz 8")}, pairs: true,
		},
		"similar names at two streets": {
			held:  NewCompany{DisplayName: brandName, Address: street("Marienplatz 8")},
			asked: NewCompany{DisplayName: lineName, Address: street("Sendlinger Straße 1")},
		},
		"a name that spells the held domain": {
			held:  NewCompany{DisplayName: "Contoso", Domains: []string{"contoso.example"}},
			asked: NewCompany{DisplayName: "contoso.example"}, pairs: true,
		},
		"a registered name matching a display name": {
			held:  NewCompany{DisplayName: "Nordlicht", LegalName: "Nordlicht Logistik GmbH"},
			asked: NewCompany{DisplayName: "Nordlicht Logistik"}, pairs: true,
		},
		"unrelated names": {held: NewCompany{DisplayName: "Fabrikam GmbH"}, asked: NewCompany{DisplayName: "Nordwind Logistik"}},
	} {
		t.Run("company: "+name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			in := CreateCompanyInput{DisplayName: tc.held.DisplayName, Source: "manual", Address: tc.held.Address}
			if tc.held.LegalName != "" {
				in.LegalName = &tc.held.LegalName
			}
			for _, domain := range tc.held.Domains {
				in.Domains = append(in.Domains, CompanyDomainInput{Domain: domain, IsPrimary: true})
			}
			if _, err := e.store.CreateCompany(ctx, in); err != nil {
				t.Fatalf("create %s: %v", tc.held.DisplayName, err)
			}
			ladder := e.dedupeCompanyInTx(ctx, t, CompanyCandidate{
				DisplayName: tc.asked.DisplayName, LegalName: tc.asked.LegalName, Address: tc.asked.Address,
			}).Decision != DecisionNoMatch
			if batch := CompanyLookAlike(tc.asked, tc.held); batch != ladder || batch != tc.pairs {
				t.Errorf("batch half = %v, ladder = %v, want both %v", batch, ladder, tc.pairs)
			}
		})
	}
	for name, tc := range map[string]struct {
		held  NewContact
		asked ContactCandidate
		pairs bool
	}{
		"one address": {
			held:  NewContact{FullName: "Anna Schmidt", Emails: []string{"anna@twinco.example"}},
			asked: ContactCandidate{FullName: "Bo Lind", Emails: []string{"Anna@TwinCo.example"}}, pairs: true,
		},
		"similar names at one employer's domain": {
			held:  NewContact{FullName: "Anna Schmidt", Emails: []string{"anna.schmidt@twinco.example"}},
			asked: ContactCandidate{FullName: "Anna Schmitt", Emails: []string{"a.schmitt@twinco.example"}}, pairs: true,
		},
		"similar names on a mail provider": {
			held:  NewContact{FullName: "Anna Schmidt", Emails: []string{"anna.schmidt@gmail.com"}},
			asked: ContactCandidate{FullName: "Anna Schmitt", Emails: []string{"a.schmitt@gmail.com"}},
		},
		"one name at two employers": {
			held:  NewContact{FullName: "Anna Schmidt", Emails: []string{"anna@twinco.example"}},
			asked: ContactCandidate{FullName: "Anna Schmidt", Emails: []string{"anna@otherco.example"}},
		},
	} {
		t.Run("contact: "+name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			in := CreateContactInput{FullName: tc.held.FullName, Source: "manual"}
			for _, email := range tc.held.Emails {
				in.Emails = append(in.Emails, ContactEmailInput{Email: email, EmailType: "work", IsPrimary: true})
			}
			if _, err := e.store.CreateContact(ctx, in); err != nil {
				t.Fatalf("create %s: %v", tc.held.FullName, err)
			}
			ladder := e.dedupeInTx(ctx, t, tc.asked).Decision != DecisionNoMatch
			if batch := ContactLookAlike(tc.asked, tc.held); batch != ladder || batch != tc.pairs {
				t.Errorf("batch half = %v, ladder = %v, want both %v", batch, ladder, tc.pairs)
			}
		})
	}
}
