// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Search's employer arm, word by word. A query whose words are spread over a
// contact's own name and their current employer's name finds that contact.
// A first name plus the company is how a reader finds someone whose last name
// they forgot.

import (
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAFirstNameAndTheEmployersNameFindTheContact(t *testing.T) {
	e := SetupSearch(t)
	demo := seedSearchCompany(t, e, "Demo GmbH")
	alice := seedSearchContact(t, e, "Alice Müller")
	bob := seedSearchContact(t, e, "Bob Becker")
	carol := seedSearchContact(t, e, "Carol Clausen")
	for _, contact := range []ids.UUID{alice, bob, carol} {
		staffCompany(t, e, contact, demo, employmentShape{})
	}

	for _, query := range []string{"alice demo", "demo alice", "alice dem", "alice demo ", "Alice Demo GmbH", "alice & demo"} {
		for _, types := range [][]string{{"contact"}, nil} {
			page := searchWith(e.Admin(), t, e, search.Input{Query: query, Types: types, WithEmployees: true})
			hit := hitFor(page, alice)
			if hit == nil {
				t.Errorf("%q (types %v) did not find Alice: %+v", query, types, page.Hits)
				continue
			}
			if hit.WorksAt == nil || hit.WorksAt.CompanyID != demo {
				t.Errorf("%q found Alice carrying works_at %+v, want Demo GmbH", query, hit.WorksAt)
			}
			for name, other := range map[string]ids.UUID{"Bob": bob, "Carol": carol} {
				if hitFor(page, other) != nil {
					t.Errorf("%q found %s, whose name carries no word of it", query, name)
				}
			}
		}
	}

	page := searchWith(e.Admin(), t, e, search.Input{Query: "demo", Types: []string{"contact"}, WithEmployees: true})
	if got := contactIDs(page); len(got) != 3 {
		t.Errorf("the company's name alone found contacts %v, want all three employees", got)
	}
}

func TestAWordNeitherSideCarriesFindsNoOne(t *testing.T) {
	e := SetupSearch(t)
	demo := seedSearchCompany(t, e, "Demo GmbH")
	alice := seedSearchContact(t, e, "Alice Müller")
	staffCompany(t, e, alice, demo, employmentShape{})

	for _, query := range []string{"alice demo zebra", "zebra demo", "alice demo zeb"} {
		page := searchWith(e.Admin(), t, e, search.Input{Query: query, WithEmployees: true})
		if hitFor(page, alice) != nil {
			t.Errorf("%q found Alice, though neither she nor her employer carries every word", query)
		}
	}
}

// More companies carry the first name than the seed cap admits, and each ranks
// above the employer; the employer's own word still seeds it.
func TestCompaniesCarryingTheFirstNameDoNotCrowdOutTheEmployer(t *testing.T) {
	e := SetupSearch(t)
	demo := seedSearchCompany(t, e, "Demo GmbH")
	alice := seedSearchContact(t, e, "Alice Müller")
	staffCompany(t, e, alice, demo, employmentShape{})
	for range 25 {
		seedSearchCompany(t, e, "Alice Alice Alice Holding")
	}

	for _, query := range []string{"alice demo", "alice dem"} {
		page := searchWith(e.Admin(), t, e, search.Input{Query: query, Types: []string{"contact"}, WithEmployees: true})
		if hit := hitFor(page, alice); hit == nil || hit.WorksAt == nil || hit.WorksAt.CompanyID != demo {
			t.Errorf("%q found Alice as %+v, want her through Demo GmbH", query, hit)
		}
	}
}

func TestAnEndedEmploymentLendsNoName(t *testing.T) {
	e := SetupSearch(t)
	demo := seedSearchCompany(t, e, "Demo GmbH")
	now := seedSearchCompany(t, e, "Neuwerk AG")
	dora := seedSearchContact(t, e, "Dora Departed")
	staffCompany(t, e, dora, demo, employmentShape{endsInDays: new(-30)})
	staffCompany(t, e, dora, now, employmentShape{})

	if page := searchWith(e.Admin(), t, e, search.Input{Query: "dora neuwerk", WithEmployees: true}); hitFor(page, dora) == nil {
		t.Fatalf("the current employer's name did not find Dora, so this proves nothing: %+v", page.Hits)
	}
	if page := searchWith(e.Admin(), t, e, search.Input{Query: "dora demo", WithEmployees: true}); hitFor(page, dora) != nil {
		t.Error("an ended employment found Dora through the old employer's name")
	}
}

func TestTheWordWiseMatchKeepsBothRowScopes(t *testing.T) {
	e := SetupSearch(t)
	public := seedSearchCompany(t, e, "Demo GmbH")
	private := e.SeedID(t, `INSERT INTO company (id, display_name, owner_id, visibility, source, captured_by)
		VALUES ($1, 'Hinterhof AG', $2, 'owner', 'manual', 'human:x')`, e.Rep3)
	hidden := e.SeedID(t, `INSERT INTO contact (id, full_name, owner_id, visibility, source, captured_by)
		VALUES ($1, 'Hanna Hidden', $2, 'owner', 'manual', 'human:x')`, e.Rep3)
	seen := seedSearchContact(t, e, "Sven Seen")
	atPrivate := seedSearchContact(t, e, "Paula Private")
	staffCompany(t, e, hidden, public, employmentShape{})
	staffCompany(t, e, seen, public, employmentShape{})
	staffCompany(t, e, atPrivate, private, employmentShape{})

	reader := e.AsTeamRep(e.Rep1, e.Team1)
	if page := searchWith(reader, t, e, search.Input{Query: "sven demo", WithEmployees: true}); hitFor(page, seen) == nil {
		t.Fatalf("the visible employee was not found, so this proves nothing: %+v", page.Hits)
	}
	if page := searchWith(reader, t, e, search.Input{Query: "hanna demo", WithEmployees: true}); hitFor(page, hidden) != nil {
		t.Error("a contact captured privately by another rep was found through its employer")
	}
	if page := searchWith(reader, t, e, search.Input{Query: "paula hinterhof", WithEmployees: true}); hitFor(page, atPrivate) != nil {
		t.Error("a company private to another rep lent its name to its employee")
	}
}

// A contact whose own text carries every word is the contact branch's hit,
// and the employer arm does not add it a second time.
func TestAContactMatchingEveryWordItselfAppearsOnce(t *testing.T) {
	e := SetupSearch(t)
	demo := seedSearchCompany(t, e, "Demo GmbH")
	own := seedSearchContact(t, e, "Alice Demo")
	staffCompany(t, e, own, demo, employmentShape{})

	page := searchWith(e.Admin(), t, e, search.Input{Query: "alice demo", Types: []string{"contact"}, WithEmployees: true})
	if len(page.Hits) != 1 || page.Hits[0].ID != own || page.Hits[0].WorksAt != nil {
		t.Fatalf("contacts came back as %+v, want only %s, matched by its own name", page.Hits, own)
	}
}

// The same question end to end. The company, its staff and their employment
// are made through the app's API; the query is a first name and part of the company.
func TestAFirstNameAndPartOfTheCompanyFindAContactMadeThroughTheApp(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{"source": "manual", "display_name": "Demo GmbH"}, nil, &company); status != 201 {
		t.Fatalf("create company → %d", status)
	}
	staff := map[string]string{}
	for _, name := range []string{"Alice Müller", "Bob Becker"} {
		var contact contactRecord
		if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": name, "source": "manual"}, nil, &contact); status != 201 {
			t.Fatalf("create contact %s → %d", name, status)
		}
		linkEdge(t, e, AnyMap{"kind": "employment", "contact_id": contact.ID, "company_id": company.ID, "source": "manual"})
		staff[name] = contact.ID
	}

	asked := url.Values{"q": {"alice dem"}, "with_employees": {"true"}}
	var page crmcontracts.SearchResponse
	if status := e.Call(t, "GET", "/v1/search?"+asked.Encode(), nil, nil, &page); status != 200 {
		t.Fatalf("search → %d", status)
	}
	found := map[string]bool{}
	for _, hit := range page.Data {
		if hit.Type == "contact" {
			found[hit.Id.String()] = true
		}
	}
	if !found[staff["Alice Müller"]] || found[staff["Bob Becker"]] {
		t.Fatalf("contacts found %v, want Alice %s and not Bob %s", found, staff["Alice Müller"], staff["Bob Becker"])
	}
}
