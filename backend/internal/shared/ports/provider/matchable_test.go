// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provider_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// surfeRules is the shape both shipped adapters declare: a profile link on its
// own, or a last name together with a company named either way.
func surfeRules() []provider.MatchRule {
	return []provider.MatchRule{
		{AllOf: []provider.IdentifierField{provider.IdentifierLinkedInURL}},
		{
			AllOf: []provider.IdentifierField{
				provider.IdentifierFirstName,
				provider.IdentifierLastName,
			},
			AnyOf: []provider.IdentifierField{
				provider.IdentifierCompanyName,
				provider.IdentifierCompanyDomain,
			},
		},
	}
}

func TestMatchableReadsTheDeclaredRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		given provider.PersonIdentifiers
		want  bool
	}{
		{
			name:  "a profile link alone is enough",
			given: provider.PersonIdentifiers{LinkedInURL: "https://linkedin.com/in/someone"},
			want:  true,
		},
		{
			name:  "a full name with a company name",
			given: provider.PersonIdentifiers{FirstName: "Michael", LastName: "Kott", CompanyName: "CM-Equity AG"},
			want:  true,
		},
		{
			name:  "a full name with a company domain",
			given: provider.PersonIdentifiers{FirstName: "Michael", LastName: "Kott", CompanyDomain: "cm-equity.de"},
			want:  true,
		},
		{
			// The disclosure says "first and last name with company", so a
			// surname alone is not enough. Sending it anyway is the guess this
			// rule refuses to make on the customer's behalf.
			name:  "a last name and a company, with no first name",
			given: provider.PersonIdentifiers{LastName: "Kott", CompanyName: "CM-Equity AG"},
			want:  false,
		},
		{
			// The case that broke the connection: a calendar-captured contact
			// carries a name and an address and nothing else.
			name:  "a full name with no company",
			given: provider.PersonIdentifiers{FirstName: "Lars", LastName: "Jankowfsky"},
			want:  false,
		},
		{
			name:  "a company with no name",
			given: provider.PersonIdentifiers{CompanyName: "CM-Equity AG"},
			want:  false,
		},
		{
			name:  "nothing at all",
			given: provider.PersonIdentifiers{},
			want:  false,
		},
		{
			// A first name is not part of either rule, so adding one to an
			// otherwise unmatchable subject must not change the answer.
			name:  "a first name does not rescue a subject without a company",
			given: provider.PersonIdentifiers{FirstName: "Michael"},
			want:  false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.given.Matchable(surfeRules()); got != c.want {
				t.Errorf("Matchable = %v, want %v for %+v", got, c.want, c.given)
			}
		})
	}
}

// An adapter that declares no rule keeps today's behaviour: everything is
// sent. The platform must not invent a constraint the vendor never stated,
// because refusing a lookup it would have answered is the worse failure and
// leaves no trace to notice.
func TestNoRulesMatchesEverySubject(t *testing.T) {
	t.Parallel()
	if !(provider.PersonIdentifiers{}).Matchable(nil) {
		t.Error("an empty subject was refused by a provider declaring no rules, so an adapter that states " +
			"no matching constraint silently stops looking anybody up")
	}
}

// A rule naming a field nobody carries can never be satisfied. The registry
// refuses such a descriptor; this pins the behaviour it is refusing, so the
// two stay one fact.
func TestAnUnknownFieldMatchesNobody(t *testing.T) {
	t.Parallel()
	rules := []provider.MatchRule{{AllOf: []provider.IdentifierField{"middle_name"}}}
	full := provider.PersonIdentifiers{
		LinkedInURL: "https://linkedin.com/in/someone",
		FirstName:   "Michael",
		LastName:    "Kott",
		CompanyName: "CM-Equity AG",
	}
	if full.Matchable(rules) {
		t.Error("a rule on an unknown identifier matched a subject carrying every real one, so a typo " +
			"in a descriptor would widen the rule instead of narrowing it")
	}
}
