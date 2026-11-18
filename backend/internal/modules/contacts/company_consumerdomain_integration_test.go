// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A consumer-mail domain never names a company, so a typed claim of one on a
// company's domain list is refused, on create and on edit alike.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func wantConsumerDomainRefusal(t *testing.T, err error) {
	t.Helper()
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || refusal.Code != "validation_error" ||
		len(refusal.Fields) != 1 || refusal.Fields[0].Code != "consumer_mail_domain" {
		t.Fatalf("want a consumer_mail_domain field refusal, got %v", err)
	}
}

func TestCompanyDomainRefusesConsumerMail(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	_, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Webmail GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "gmail.com", IsPrimary: true}},
	})
	wantConsumerDomainRefusal(t, err)

	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Voltaq Systems GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "voltaq.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	_, err = e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{
		Domains: &[]CompanyDomainInput{
			{Domain: "voltaq.test", IsPrimary: true},
			{Domain: "https://www.GMAIL.com/", IsPrimary: false},
		},
	})
	wantConsumerDomainRefusal(t, err)
	if live := liveDomainsOf(ctx, t, e, companyID); len(live) != 1 || !live["voltaq.test"] {
		t.Fatalf("a refused edit changed the domains: %+v", live)
	}
}
