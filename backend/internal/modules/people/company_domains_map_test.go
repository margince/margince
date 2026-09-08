// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

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
