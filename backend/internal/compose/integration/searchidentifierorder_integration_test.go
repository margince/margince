// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func identifierCompany(t *testing.T, e *Env, name, domain string, employee bool) ids.UUID {
	t.Helper()
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: name, Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(company.Id)
	if employee {
		member := ids.From[ids.ContactKind](seedContactWithEmail(e.Admin(), t, e, "Unrelated contact at "+name, "contact-"+id.String()+"@staff.test", nil))
		employer := ids.From[ids.CompanyKind](id)
		if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &member, CompanyID: &employer, Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestAnAddressPrefixRanksItsLeadAndCompanyBeforeDomainFragments(t *testing.T) {
	e := Setup(t)
	for _, domain := range []string{"examplecorp.net", "exampleconsulting.net", "examplecommerce.net"} {
		identifierCompany(t, e, domain, domain, true)
	}
	target := identifierCompany(t, e, "The right company", "example.com", false)
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Lena"), Email: new("lena@example.com"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	perType := 3
	page, err := search.NewStore(e.DB()).Search(e.Admin(), search.Input{AddressCompanies: true, Query: "lena@exam", PerType: &perType, WithEmployees: true})
	if err != nil {
		t.Fatal(err)
	}
	var companies []ids.UUID
	foundLead := false
	for _, hit := range page.Hits {
		if hit.Type == "contact" {
			t.Fatalf("domain fragment seeded an unrelated contact: %+v", hit)
		}
		if hit.Type == "company" {
			companies = append(companies, hit.ID)
		}
		if hit.Type == "lead" && hit.ID == ids.UUID(lead.Id) {
			foundLead = true
			if hit.Score < 8 {
				t.Fatalf("address prefix has no ranking evidence: %+v", hit)
			}
		}
	}
	if !foundLead || len(companies) != 3 || companies[0] != target {
		t.Fatalf("lead=%v companies=%v want %s first", foundLead, companies, target)
	}
}

func TestDomainClosenessOrdersBothGroupedAndRankedPages(t *testing.T) {
	e := Setup(t)
	identifierCompany(t, e, "Old long account", "exampleconsulting.net", false)
	nearest := identifierCompany(t, e, "New close account", "example.com", false)
	store := search.NewStore(e.DB())
	perType := 3
	grouped, err := store.Search(e.Admin(), search.Input{AddressCompanies: true, Query: "exam", Types: []string{"company"}, PerType: &perType})
	if err != nil {
		t.Fatal(err)
	}
	if len(grouped.Hits) != 2 || grouped.Hits[0].ID != nearest {
		t.Fatalf("closeness did not precede identity: %+v", grouped.Hits)
	}
	in := search.Input{AddressCompanies: true, Query: "exam", Types: []string{"company"}, Limit: 1}
	var seen []ids.UUID
	for range 3 {
		page, err := store.Search(e.Admin(), in)
		if err != nil {
			t.Fatal(err)
		}
		for _, hit := range page.Hits {
			seen = append(seen, hit.ID)
		}
		if !page.HasMore {
			break
		}
		in.Cursor = page.NextCursor
	}
	if len(seen) != 2 || seen[0] != nearest || seen[0] == seen[1] {
		t.Fatalf("ranked continuation=%v", seen)
	}
}

func TestAMatchedAddressOutranksACloserUnrelatedDomain(t *testing.T) {
	e := Setup(t)
	identifierCompany(t, e, "Closer fragment", "exam.io", false)
	target := identifierCompany(t, e, "Contact's employer", "exampleinternationalconsulting.com", false)
	if _, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Lena"), Email: new("lena@exampleinternationalconsulting.com"), Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	page, err := search.NewStore(e.DB()).Search(e.Admin(), search.Input{AddressCompanies: true, Query: "lena@exam", Types: []string{"company"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != target {
		t.Fatalf("address association lost: %+v", page.Hits)
	}
}

func TestADomainFragmentDoesNotSeedEmployeesThroughTheNameIndex(t *testing.T) {
	e := Setup(t)
	identifierCompany(t, e, "Neutral account", "exampleconsulting.net", true)
	store := search.NewStore(e.DB())
	fragment, err := store.Search(e.Admin(), search.Input{AddressCompanies: true, Query: "exam", Types: []string{"contact"}, WithEmployees: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(fragment.Hits) != 0 {
		t.Fatalf("domain fragment seeded staff: %+v", fragment.Hits)
	}
	exact, err := store.Search(e.Admin(), search.Input{AddressCompanies: true, Query: "exampleconsulting.net", Types: []string{"contact"}, WithEmployees: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(exact.Hits) != 1 || exact.Hits[0].WorksAt == nil {
		t.Fatalf("exact domain lost staff: %+v", exact.Hits)
	}
}

func TestAnUnreadableSubjectCannotPromoteACompanyMatch(t *testing.T) {
	e := Setup(t)
	nearest := identifierCompany(t, e, "Close account", "exam.io", false)
	associated := identifierCompany(t, e, "Hidden contact's company", "exampleinternationalconsulting.com", false)
	owner := ids.From[ids.UserKind](e.Rep3)
	if _, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Private prospect"), Email: new("lena@exampleinternationalconsulting.com"), OwnerID: &owner, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	for _, readLead := range []bool{false, true} {
		ctx := e.As(e.AdminUser, nil, principal.Permissions{Objects: map[string]principal.ObjectGrant{"company": {Read: true}, "lead": {Read: readLead}}, RowScope: principal.RowScopeOwn})
		page, err := search.NewStore(e.DB()).Search(ctx, search.Input{AddressCompanies: true, Query: "lena@exam", Types: []string{"company"}, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		want := nearest
		if readLead {
			want = associated
		}
		if len(page.Hits) != 1 || page.Hits[0].ID != want {
			t.Fatalf("subject visibility and company ranking disagree (read=%v): %+v", readLead, page.Hits)
		}
	}
}

func TestAPrivateContactsAddressDoesNotBoostItsCompany(t *testing.T) {
	e := Setup(t)
	nearest := identifierCompany(t, e, "Close account", "exam.io", false)
	identifierCompany(t, e, "Private contact's company", "exampleinternationalconsulting.com", false)
	contact := seedContactWithEmail(e.As(e.Rep3, nil, AdminPerms), t, e, "Private contact", "pat@exampleinternationalconsulting.com", nil)
	e.MakeCapturePrivate(t, "contact", contact, e.Rep3)
	ctx := e.As(e.AdminUser, nil, principal.Permissions{Objects: map[string]principal.ObjectGrant{"company": {Read: true}, "contact": {Read: true}}, RowScope: principal.RowScopeAll})
	page, err := search.NewStore(e.DB()).Search(ctx, search.Input{AddressCompanies: true, Query: "pat@exam", Types: []string{"company"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != nearest {
		t.Fatalf("private contact boosted company: %+v", page.Hits)
	}
}

func TestIdentifierTiersCarryEvidenceAcrossRankedPages(t *testing.T) {
	e := Setup(t)
	exact := seedContactWithEmail(e.Admin(), t, e, "Unrelated exact", "a_b%tag@example.com", nil)
	prefix := seedContactWithEmail(e.Admin(), t, e, "Unrelated prefix", "a_b%tag@example.com.au", nil)
	seedContactWithEmail(e.Admin(), t, e, "Wildcard trap", "axbytag@example.com", nil)
	store := search.NewStore(e.DB())
	in := search.Input{Query: "A_B%tag@EXAMPLE.com", Types: []string{"contact"}, Limit: 1}
	for _, want := range []struct {
		id    ids.UUID
		match crmcontracts.SearchResultIdentifierMatch
		tier  float64
	}{
		{exact, crmcontracts.SearchResultIdentifierMatchExact, 16},
		{prefix, crmcontracts.SearchResultIdentifierMatchPrefix, 8},
	} {
		page, err := store.Search(e.Admin(), in)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Hits) != 1 {
			t.Fatalf("page = %+v", page)
		}
		hit := page.Hits[0]
		if hit.ID != want.id || hit.IdentifierMatch == nil || *hit.IdentifierMatch != want.match || hit.Score < want.tier || hit.Score >= want.tier+2 {
			t.Fatalf("hit %+v, want %+v", hit, want)
		}
		in.Cursor = page.NextCursor
	}
}

func TestOnlyInteractiveSearchUsesAddressCompanyAssociation(t *testing.T) {
	e := Setup(t)
	target := identifierCompany(t, e, "Unrelated account", "exampleinternationalconsulting.com", false)
	seedContactWithEmail(e.Admin(), t, e, "Lena", "lena@exampleinternationalconsulting.com", nil)
	store := search.NewStore(e.DB())
	for _, interactive := range []bool{false, true} {
		page, err := store.Search(e.Admin(), search.Input{Query: "lena@exam", Types: []string{"company"}, AddressCompanies: interactive})
		if err != nil {
			t.Fatal(err)
		}
		hit := hitFor(page, target)
		if interactive && (hit == nil || hit.Score < 4 || hit.Score >= 8 || hit.IdentifierMatch != nil) {
			t.Fatalf("association = %+v", hit)
		}
		if !interactive && hit != nil && hit.Score >= 4 {
			t.Fatalf("retrieval inherited address evidence: %+v", hit)
		}
	}
}

func TestAnApostropheInACompanyNameStillSeedsItsEmployees(t *testing.T) {
	e := Setup(t)
	company := identifierCompany(t, e, "O'Reilly", "neutral-company.test", false)
	member := ids.From[ids.ContactKind](seedContactWithEmail(e.Admin(), t, e, "Mira Voss", "mira@independent.test", nil))
	employer := ids.From[ids.CompanyKind](company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &member, CompanyID: &employer, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	page, err := search.NewStore(e.DB()).Search(e.Admin(), search.Input{Query: "oreilly", Types: []string{"contact"}, WithEmployees: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].WorksAt == nil || page.Hits[0].WorksAt.CompanyID != company || page.Hits[0].IdentifierMatch != nil {
		t.Fatalf("company name lost its staff: %+v", page.Hits)
	}
}

func TestAMatchedAddressUsesItsStoredFoldedDomain(t *testing.T) {
	e := Setup(t)
	target := identifierCompany(t, e, "Independent account", "xn--bcher-kva.example", false)
	if _, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Email: new("lena@bücher.example"), FullName: new("Lena"), Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	page, err := search.NewStore(e.DB()).Search(e.Admin(), search.Input{Query: "lena@", Types: []string{"company"}, AddressCompanies: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != target || page.Hits[0].IdentifierMatch != nil {
		t.Fatalf("folded-domain company link = %+v", page.Hits)
	}
}
