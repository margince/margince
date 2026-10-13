// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Search finds a contact by a pasted address and a company by its domain. The
// name index holds neither, so these rows reach search only through the
// identifier arm the lists share.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

func TestSearchFindsAContactByItsEmailAddress(t *testing.T) {
	e := Setup(t)
	anna := seedContactWithEmail(e.Admin(), t, e, "Anna Schmidt", "anna@example.com", nil)
	store := search.NewStore(e.DB())
	perType := 5

	for _, in := range []search.Input{
		{Query: "anna@example.com"},
		{Query: "anna@example.com", Types: []string{"contact"}},
		{Query: "Anna@Example.COM ", PerType: &perType},
		// The palette's own request: grouped, with the employer arm.
		{Query: "anna@example.com", PerType: &perType, WithEmployees: true},
	} {
		page, err := store.Search(e.Admin(), in)
		if err != nil {
			t.Fatalf("search %+v: %v", in, err)
		}
		if len(page.Hits) != 1 || page.Hits[0].ID != anna {
			t.Errorf("search %+v found %+v, want the contact %s alone", in, page.Hits, anna)
			continue
		}
		if page.Hits[0].Score < 1 {
			t.Errorf("search %+v scores the address hit %v, want the whole-word tier (at least 1)", in, page.Hits[0].Score)
		}
	}
}

// A classification judges what the search finds, so the address that finds a
// contact also matches it there.
func TestAClassificationMatchesAContactByItsAddress(t *testing.T) {
	e := Setup(t)
	anna := seedContactWithEmail(e.Admin(), t, e, "Anna Schmidt", "anna@example.com", nil)
	other := seedContactWithEmail(e.Admin(), t, e, "Bruno Becker", "bruno@example.com", nil)

	got, err := search.NewRetriever(search.NewStore(e.DB()), nil).Classify(e.Admin(), retrieval.ClassifyQuery{
		Text: "anna@example.com", EntityType: datasource.EntityContact, Within: []ids.UUID{anna, other},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matched) != 1 || got.Matched[0] != anna || len(got.Unmatched) != 1 || got.Unmatched[0] != other {
		t.Fatalf("classification = %+v, want %s matched and %s unmatched", got, anna, other)
	}
}

// A name search ranks as it did. A hit found by its name alone gets no
// identifier lift, so a prefix match stays below the whole word.
func TestANameSearchKeepsItsRanking(t *testing.T) {
	e := Setup(t)
	philip := seedContactWithEmail(e.Admin(), t, e, "Philip Dorn", "philip@dorn.example", nil)
	philipp := seedContactWithEmail(e.Admin(), t, e, "Philipp Adler", "philipp@adler.example", nil)

	page, err := search.NewStore(e.DB()).Search(e.Admin(), search.Input{Query: "philip", Types: []string{"contact"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 2 || page.Hits[0].ID != philip || page.Hits[1].ID != philipp {
		t.Fatalf("hits = %+v, want Philip Dorn then Philipp Adler", page.Hits)
	}
	if page.Hits[1].Score >= 1 {
		t.Errorf("the prefix-only match scores %v, want below the whole-word tier", page.Hits[1].Score)
	}
}

func TestSearchFindsACompanyByItsDomain(t *testing.T) {
	e := Setup(t)
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Quedlinburg Werke",
		Domains:     []contacts.CompanyDomainInput{{Domain: "quedlinburg-werke.de", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the company: %v", err)
	}
	store := search.NewStore(e.DB())
	perType := 5

	for _, in := range []search.Input{
		{Query: "Quedlinburg-Werke.de", Types: []string{"company"}},
		{Query: "quedlinburg-werke.de", PerType: &perType, WithEmployees: true},
	} {
		page, err := store.Search(e.Admin(), in)
		if err != nil {
			t.Fatalf("search %+v: %v", in, err)
		}
		if len(page.Hits) != 1 || page.Hits[0].ID != ids.UUID(company.Id) {
			t.Errorf("search %+v found %+v, want the company %s alone", in, page.Hits, company.Id)
		}
	}
}

// An address is a read of its contact. A contact the reader may not see stays
// hidden even when its whole address is typed.
func TestAnAddressFindsNoContactOutsideTheReadersScope(t *testing.T) {
	e := Setup(t)
	owner := e.As(e.Rep3, []ids.UUID{e.Team2}, RepPerms)
	private := seedContactWithEmail(owner, t, e, "Hanna Hidden", "hanna@hidden.example", &e.Rep3)
	visibility := "owner"
	if _, err := e.Contacts.UpdateContact(owner, ids.From[ids.ContactKind](private),
		contacts.UpdateContactInput{Visibility: &visibility}); err != nil {
		t.Fatalf("making the contact private: %v", err)
	}
	store := search.NewStore(e.DB())
	in := search.Input{Query: "hanna@hidden.example"}

	page, err := store.Search(owner, in)
	if err != nil {
		t.Fatal(err)
	}
	if hitFor(page, private) == nil {
		t.Fatalf("the owner does not find their own contact by its address, so this proves nothing: %+v", page.Hits)
	}
	page, err = store.Search(e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 0 {
		t.Errorf("a rep outside the contact's scope found %+v by its address", page.Hits)
	}
}

// An address the contact no longer carries finds nothing; the live one does.
func TestAnArchivedAddressFindsNothing(t *testing.T) {
	e := Setup(t)
	moved := seedContactWithEmail(e.Admin(), t, e, "Max Moved", "max@old.example", nil)
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](moved), contacts.UpdateContactInput{
		Emails: []contacts.ContactEmailInput{{Email: "max@new.example", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("replacing the address: %v", err)
	}
	store := search.NewStore(e.DB())

	page, err := store.Search(e.Admin(), search.Input{Query: "max@old.example"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 0 {
		t.Errorf("the archived address found %+v", page.Hits)
	}
	page, err = store.Search(e.Admin(), search.Input{Query: "max@new.example"})
	if err != nil {
		t.Fatal(err)
	}
	if hitFor(page, moved) == nil {
		t.Errorf("the live address did not find the contact: %+v", page.Hits)
	}
}

// seedContactWithEmail creates a contact with one work address through the
// contact writer, as the seat in ctx.
func seedContactWithEmail(ctx context.Context, t *testing.T, e *Env, name, email string, owner *ids.UUID) ids.UUID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
		FullName: name, OwnerID: userIDPtr(owner), Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return ids.UUID(contact.Id)
}

// A contact both the contact branch and the employer arm could return is the
// contact branch's: one hit, carrying no employer.
func TestAContactFoundByAddressIsNotRepeatedThroughItsEmployer(t *testing.T) {
	e := Setup(t)
	anna := seedContactWithEmail(e.Admin(), t, e, "Anna Schmidt", "anna@example.com", nil)
	employer := e.SeedCompany(t, "anna@example.com Holdings", nil)
	contactID, companyID := ids.From[ids.ContactKind](anna), ids.From[ids.CompanyKind](employer)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
	}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}

	page, err := search.NewStore(e.DB()).Search(e.Admin(),
		search.Input{Query: "anna@example.com", Types: []string{"contact"}, WithEmployees: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != anna || page.Hits[0].WorksAt != nil {
		t.Fatalf("hits = %+v, want the contact once, found by its own address", page.Hits)
	}
}

// A pasted domain seeds the employer arm like the company's name does, so the
// palette lists the contacts who work there.
func TestADomainFindsTheCompanysEmployees(t *testing.T) {
	e := Setup(t)
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Quedlinburg Werke",
		Domains:     []contacts.CompanyDomainInput{{Domain: "quedlinburg-werke.de", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the company: %v", err)
	}
	ines := seedContactWithEmail(e.Admin(), t, e, "Ines Kurz", "ines@kurz.example", nil)
	contactID, companyID := ids.From[ids.ContactKind](ines), ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
	}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}

	page, err := search.NewStore(e.DB()).Search(e.Admin(),
		search.Input{Query: "quedlinburg-werke.de", Types: []string{"contact"}, WithEmployees: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != ines || page.Hits[0].WorksAt == nil {
		t.Fatalf("hits = %+v, want the employee through the company's domain", page.Hits)
	}
}

// A contact whose name holds no searchable word is still judged when its
// address matches, because the search returns it.
func TestAClassificationJudgesAContactFoundOnlyByItsAddress(t *testing.T) {
	e := Setup(t)
	nameless := seedContactWithEmail(e.Admin(), t, e, "🙂", "smile@example.com", nil)

	got, err := search.NewRetriever(search.NewStore(e.DB()), nil).Classify(e.Admin(), retrieval.ClassifyQuery{
		Text: "smile@example.com", EntityType: datasource.EntityContact, Within: []ids.UUID{nameless},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matched) != 1 || got.Matched[0] != nameless {
		t.Fatalf("classification = %+v, want %s matched", got, nameless)
	}
}
