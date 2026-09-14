// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The pure edges of the domain-edit path: the wire→input mapper folds the
// replace-set (nil stays untouched), and the single-primary check is the
// uq_company_domain_primary invariant expressed as a typed 409 up front.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestCompanyUpdateInputMapsDomains(t *testing.T) {
	primary := true
	req := crmcontracts.UpdateCompanyRequest{
		Domains: &[]crmcontracts.CompanyDomainInput{
			{Domain: "a.test", IsPrimary: &primary},
			{Domain: "b.test"},
		},
	}
	in := companyUpdateInput(req, nil)
	if in.Domains == nil || len(*in.Domains) != 2 {
		t.Fatalf("domains not mapped: %+v", in.Domains)
	}
	if (*in.Domains)[0] != (CompanyDomainInput{Domain: "a.test", IsPrimary: true}) {
		t.Fatalf("first domain = %+v, want {a.test true}", (*in.Domains)[0])
	}
	if (*in.Domains)[1].IsPrimary {
		t.Fatalf("second domain should not be primary: %+v", (*in.Domains)[1])
	}

	// Absent domains stay nil — a scalar-only edit must not touch them.
	if in2 := companyUpdateInput(crmcontracts.UpdateCompanyRequest{}, nil); in2.Domains != nil {
		t.Fatalf("absent domains must stay nil (untouched), got %+v", in2.Domains)
	}

	// An explicitly-present empty array is the "clear all" replace-set — a
	// non-nil empty slice, distinct from absent/nil.
	empty := companyUpdateInput(crmcontracts.UpdateCompanyRequest{
		Domains: &[]crmcontracts.CompanyDomainInput{},
	}, nil)
	if empty.Domains == nil || len(*empty.Domains) != 0 {
		t.Fatalf("empty domains array must map to a non-nil empty slice (clear-all), got %+v", empty.Domains)
	}
}

func TestDedupeDomains(t *testing.T) {
	got := dedupeDomains([]CompanyDomainInput{
		{Domain: "acme.test", IsPrimary: false},
		{Domain: "acme.test", IsPrimary: true},
		{Domain: "b.test"},
	})
	if len(got) != 2 {
		t.Fatalf("dedupe collapsed to %d rows, want 2: %+v", len(got), got)
	}
	if got[0].Domain != "acme.test" || !got[0].IsPrimary {
		t.Fatalf("collapsed acme.test must keep primary=true (OR of occurrences): %+v", got[0])
	}
}

// A domain set that names no primary is the state the contract admits and no
// reader can act on, so the election fills it. The cases below are the four
// answers it has to tell apart; the create path passes "" for current and the
// edit path passes the live primary.
func TestElectPrimary(t *testing.T) {
	primaryOf := func(t *testing.T, got []CompanyDomainInput) string {
		t.Helper()
		primary := ""
		for _, d := range got {
			if !d.IsPrimary {
				continue
			}
			if primary != "" {
				t.Fatalf("elected two primaries: %+v", got)
			}
			primary = d.Domain
		}
		return primary
	}

	t.Run("silence elects the first domain", func(t *testing.T) {
		got := electPrimary([]CompanyDomainInput{{Domain: "a.test"}, {Domain: "b.test"}}, "")
		if p := primaryOf(t, got); p != "a.test" {
			t.Fatalf("elected %q, want a.test", p)
		}
	})

	t.Run("a sole domain is the case the sweep needs", func(t *testing.T) {
		got := electPrimary([]CompanyDomainInput{{Domain: "acme.test"}}, "")
		if p := primaryOf(t, got); p != "acme.test" {
			t.Fatalf("elected %q, want acme.test", p)
		}
	})

	t.Run("a caller who named one is obeyed", func(t *testing.T) {
		got := electPrimary([]CompanyDomainInput{{Domain: "a.test"}, {Domain: "b.test", IsPrimary: true}}, "")
		if p := primaryOf(t, got); p != "b.test" {
			t.Fatalf("elected %q, want the caller's b.test", p)
		}
	})

	// The edit case, and the reason current is a parameter at all: adding a
	// domain to a record must not move the primary the record already had.
	t.Run("the live primary is kept when it survives the edit", func(t *testing.T) {
		got := electPrimary([]CompanyDomainInput{{Domain: "a.test"}, {Domain: "b.test"}}, "b.test")
		if p := primaryOf(t, got); p != "b.test" {
			t.Fatalf("elected %q, want the live b.test", p)
		}
	})

	t.Run("a live primary the edit removes falls back to the first", func(t *testing.T) {
		got := electPrimary([]CompanyDomainInput{{Domain: "a.test"}, {Domain: "b.test"}}, "gone.test")
		if p := primaryOf(t, got); p != "a.test" {
			t.Fatalf("elected %q, want a.test", p)
		}
	})

	// Clearing every domain is a real answer, and electing into an empty set
	// would invent a row the caller did not ask for.
	t.Run("an empty set elects nothing", func(t *testing.T) {
		if got := electPrimary(nil, ""); len(got) != 0 {
			t.Fatalf("elected %+v from nothing", got)
		}
	})

	// The input is the caller's slice, and the create path hands it one it
	// still holds. Writing through it would move a primary in the caller's own
	// copy of the request.
	t.Run("the caller's slice is not written through", func(t *testing.T) {
		in := []CompanyDomainInput{{Domain: "a.test"}}
		electPrimary(in, "")
		if in[0].IsPrimary {
			t.Fatal("electPrimary wrote its election back into the caller's slice")
		}
	})
}

func TestSingleDesiredPrimary(t *testing.T) {
	if p, err := singleDesiredPrimary([]CompanyDomainInput{{Domain: "a.test"}, {Domain: "b.test"}}); err != nil || p != "" {
		t.Fatalf("no primary → (%q, %v), want ('', nil)", p, err)
	}
	if p, err := singleDesiredPrimary([]CompanyDomainInput{{Domain: "a.test", IsPrimary: true}, {Domain: "b.test"}}); err != nil || p != "a.test" {
		t.Fatalf("one primary → (%q, %v), want ('a.test', nil)", p, err)
	}
	if _, err := singleDesiredPrimary([]CompanyDomainInput{{Domain: "a.test", IsPrimary: true}, {Domain: "b.test", IsPrimary: true}}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("two primaries → err %v, want ErrConflict", err)
	}
}

func TestDomainSummaries(t *testing.T) {
	got := domainSummaries([]CompanyDomainInput{{Domain: "a.test", IsPrimary: true}, {Domain: "b.test"}})
	if len(got) != 2 || got[0]["domain"] != "a.test" || got[0]["is_primary"] != true || got[1]["is_primary"] != false {
		t.Fatalf("domainSummaries = %+v", got)
	}
}
