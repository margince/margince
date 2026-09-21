// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A pasted URL and a bare host select the same account on BOTH surfaces that
// filter companies by domain.
//
// The list parameter has folded its value all along — a caller who pasted
// `https://www.acme.example/careers` out of an email signature is asking about
// `acme.example`, which is what the column holds. A segment leaf did not, so
// one product fact answered two different questions depending on which surface
// asked: the list found the account and the saved view found nothing.
//
// ONE test over both, because "the same answer" is the claim. Two tests would
// each keep passing while the surfaces drifted apart, which is the state this
// was filed about.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestADomainFilterAnswersTheSameAccountOnBothSurfaces(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	lists := collections.NewStore(e.DB())

	acme := seedCompanyWithDomain(t, e, "Acme", "acme.example")
	seedCompanyWithDomain(t, e, "Other", "other.example")

	// The three spellings of one question: what the column holds, the same
	// with a subdomain and a path, and a difference of case alone.
	for _, asked := range []string{
		"acme.example",
		"https://www.acme.example/careers",
		"ACME.Example",
	} {
		t.Run(asked, func(t *testing.T) {
			listed, _, err := e.Contacts.ListCompanies(admin, contacts.ListCompaniesInput{Domain: &asked})
			if err != nil {
				t.Fatalf("the list parameter refused %q: %v", asked, err)
			}
			if len(listed) != 1 || ids.UUID(listed[0].Id) != acme {
				t.Fatalf("the list answered %d account(s) for %q, want only Acme", len(listed), asked)
			}

			segment, err := lists.CreateList(admin, collections.CreateListInput{
				Name: "by domain " + asked, EntityType: "company", ListType: "dynamic",
				Definition: map[string]any{"field": "domain", "op": "eq", "value": asked},
			})
			if err != nil {
				t.Fatalf("storing a segment on %q: %v — the leaf refuses a value the list accepts, "+
					"so a reader can filter by domain on one surface and not the other", asked, err)
			}
			members, _, err := lists.ListMembers(admin, segment.ID, 50, "")
			if err != nil {
				t.Fatalf("evaluating the segment: %v", err)
			}
			if len(members) != 1 || members[0].EntityID != acme {
				t.Fatalf("the segment answered %d member(s) for %q, want the same one account the "+
					"list answered", len(members), asked)
			}
		})
	}
}

// A value no domain can be read from is REFUSED rather than folded to empty.
// Empty would bind and match nothing, which reads to a caller exactly like a
// domain nobody uses — and in a saved view the mistake would stay invisible.
func TestASegmentRefusesAValueThatIsNotADomain(t *testing.T) {
	e := Setup(t)
	lists := collections.NewStore(e.DB())

	_, err := lists.CreateList(e.Admin(), collections.CreateListInput{
		Name: "not a domain", EntityType: "company", ListType: "dynamic",
		Definition: map[string]any{"field": "domain", "op": "eq", "value": "not a domain at all"},
	})
	if err == nil {
		t.Fatal("a segment stored a domain leaf whose value is not a domain; it would match nothing " +
			"and read as an account nobody has")
	}
}

// seedCompanyWithDomain creates an account carrying one domain, through the real
// writer — the column the leaf reads is written by CreateCompany's own
// domain path, and a fixture inserting the row itself would prove nothing about
// the form that path stores.
func seedCompanyWithDomain(t *testing.T, e *Env, name, domain string) ids.UUID {
	t.Helper()
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: name,
		Domains:     []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}},
		Source:      "manual",
	})
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return ids.UUID(company.Id)
}
